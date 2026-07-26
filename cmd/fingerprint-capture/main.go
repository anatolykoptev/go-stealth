// Command fingerprint-capture downloads a Chrome-for-Testing build matching a
// given major version, runs it against the peet.ws echo endpoint, and writes the
// observed fingerprint to testdata/reference_chrome_<major>.json with full
// provenance.
//
// This is the reference-capture half of go-stealth's fingerprint oracle. The
// oracle test (//go:build fingerprint) compares go-stealth's emitted
// fingerprint against these references.
//
// Usage:
//
//	go run ./cmd/fingerprint-capture -major 146
//	go run ./cmd/fingerprint-capture -major 146 -mode headful -display :99
//
// Why a cmd/ program and not a build-tagged helper: the capture tool is a CLI
// that downloads and runs an external browser binary — a genuine standalone
// program, not library-internal test machinery. Go convention puts CLIs in
// cmd/<name>/. The repo is otherwise flat, but introducing one cmd/ subdir for a
// real CLI is the idiomatic Go layout, and it keeps the capture tool out of the
// stealth package's surface area. It compiles under `go build ./...` (preflight)
// but never runs there — main packages have no test entry point.
package main

import (
	"archive/zip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/anatolykoptev/go-stealth/internal/fingerprint"
)

const versionsJSON = "https://googlechromelabs.github.io/chrome-for-testing/known-good-versions-with-downloads.json"

func main() {
	var (
		major       = flag.String("major", "", "Chrome major version to capture (e.g. 146), required")
		endpoint    = flag.String("endpoint", fingerprint.Endpoint, "echo endpoint URL")
		cacheDir    = flag.String("cache-dir", defaultCacheDir(), "browser download cache (under /home/krolik/tmp by default)")
		testdataDir = flag.String("testdata-dir", "testdata", "where to write the reference JSON")
		mode        = flag.String("mode", "headless", "capture mode: headless | headful")
		display     = flag.String("display", "", "X display for headful mode (e.g. :99); requires Xvfb")
	)
	flag.Parse()
	if *major == "" {
		fmt.Fprintln(os.Stderr, "fingerprint-capture: -major is required")
		os.Exit(2)
	}

	if err := run(*major, *endpoint, *cacheDir, *testdataDir, *mode, *display); err != nil {
		fmt.Fprintf(os.Stderr, "fingerprint-capture: %v\n", err)
		os.Exit(1)
	}
}

func defaultCacheDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, "tmp", "go-stealth-fingerprint-cache")
	}
	return filepath.Join(os.TempDir(), "go-stealth-fingerprint-cache")
}

func run(major, endpoint, cacheDir, testdataDir, mode, display string) error {
	if runtime.GOARCH == "arm64" {
		return fmt.Errorf(
			"Chrome-for-Testing has no linux-arm64 build; cannot capture a matching-major reference on aarch64. " +
				"Run on an amd64 host, or capture references elsewhere and commit them to testdata/. " +
				"Using the box's snap Chromium is NOT a substitute: it is Chromium (not Chrome, so its sec-ch-ua brand list differs) and its major will not match the profile major, so the oracle would report version drift as if it were a profile bug.")
	}

	fmt.Fprintf(os.Stderr, "resolving Chrome-for-Testing %s linux64...\n", major)
	binPath, version, err := ensureChrome(major, cacheDir)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "using chrome %s at %s\n", version, binPath)

	body, err := captureWithChrome(binPath, endpoint, mode, display)
	if err != nil {
		return fmt.Errorf("capture: %w", err)
	}

	raw, err := fingerprint.ParsePeet(body)
	if err != nil {
		return err
	}
	obs, err := fingerprint.ExtractObserved(raw)
	if err != nil {
		return err
	}

	ref := &fingerprint.Reference{
		Browser:                "Chrome",
		BrowserVersion:         version,
		Major:                  major,
		CaptureTime:            time.Now().UTC(),
		Endpoint:               endpoint,
		Mode:                   mode,
		Arch:                   runtime.GOARCH,
		BrowserSource:          "chrome-for-testing",
		TLS:                    obs.TLS,
		HTTP2AkamaiFingerprint: obs.HTTP2AkamaiFingerprint,
		HeaderOrder:            obs.HeaderOrder,
		Accept:                 obs.Accept,
		AcceptLanguage:         obs.AcceptLanguage,
		AcceptEncoding:         obs.AcceptEncoding,
	}

	if mode == "headless" {
		// Headless Chrome reports HeadlessChrome/<ver> and its sec-ch-ua brand
		// list uses "HeadlessChrome" instead of "Google Chrome" — NOT a valid
		// reference for those two fields. Exclude them with an explicit note so
		// the oracle skips them with a reason rather than silently.
		ref.Notes = append(ref.Notes,
			"headless mode excludes user-agent: HeadlessChrome/<ver> != Chrome/<ver>",
			"headless mode excludes sec-ch-ua: brand list uses HeadlessChrome, not Google Chrome",
		)
	} else {
		ref.SecChUa = obs.SecChUa
		ref.SecChUaMobile = obs.SecChUaMobile
		ref.SecChUaPlatform = obs.SecChUaPlatform
		ref.UserAgent = obs.UserAgent
	}

	if err := os.MkdirAll(testdataDir, 0o755); err != nil {
		return err
	}
	out := fingerprint.ReferencePath(testdataDir, major)
	if err := fingerprint.SaveReference(out, ref); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", out)
	return nil
}

// ensureChrome resolves the Chrome-for-Testing linux64 build for the given major,
// downloads + unzips it into cacheDir if not already cached, and returns the
// path to the chrome binary and its full version string.
func ensureChrome(major, cacheDir string) (binPath, version string, err error) {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", "", err
	}
	ver, url, err := resolveVersion(major)
	if err != nil {
		return "", "", err
	}
	installDir := filepath.Join(cacheDir, "chrome-"+ver)
	bin := filepath.Join(installDir, "chrome-linux64", "chrome")
	if _, err := os.Stat(bin); err == nil {
		return bin, ver, nil
	}

	fmt.Fprintf(os.Stderr, "downloading %s...\n", url)
	zipPath := filepath.Join(cacheDir, "chrome-"+ver+".zip")
	if err := download(url, zipPath); err != nil {
		return "", "", err
	}
	defer os.Remove(zipPath)
	if err := unzipTo(zipPath, installDir); err != nil {
		return "", "", err
	}
	if _, err := os.Stat(bin); err != nil {
		return "", "", fmt.Errorf("chrome binary not found after unzip: %w", err)
	}
	return bin, ver, nil
}

type versionsDoc struct {
	Versions []struct {
		Version   string `json:"version"`
		Downloads struct {
			Chrome []struct {
				Platform string `json:"platform"`
				URL      string `json:"url"`
			} `json:"chrome"`
		} `json:"downloads"`
	} `json:"versions"`
}

// resolveVersion finds the latest Chrome-for-Testing version whose major matches
// and returns its linux64 download URL.
func resolveVersion(major string) (version, url string, err error) {
	resp, err := http.Get(versionsJSON)
	if err != nil {
		return "", "", fmt.Errorf("fetch versions JSON: %w", err)
	}
	defer resp.Body.Close()
	var doc versionsDoc
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return "", "", fmt.Errorf("decode versions JSON: %w", err)
	}
	var found string
	for _, v := range doc.Versions {
		if strings.HasPrefix(v.Version, major+".") {
			found = v.Version // iterate in order; last match = latest
		}
	}
	if found == "" {
		return "", "", fmt.Errorf("no Chrome-for-Testing version found for major %s", major)
	}
	for _, v := range doc.Versions {
		if v.Version != found {
			continue
		}
		for _, d := range v.Downloads.Chrome {
			if d.Platform == "linux64" {
				return found, d.URL, nil
			}
		}
	}
	return "", "", fmt.Errorf("no linux64 download for Chrome-for-Testing %s", found)
}

func download(url, dst string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

func unzipTo(zipPath, dst string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, f := range r.File {
		path := filepath.Join(dst, f.Name)
		if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(dst)+string(os.PathSeparator)) {
			return fmt.Errorf("zip slip: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(path, 0o755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			out.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// captureWithChrome runs the chrome binary against the endpoint and returns the
// raw peet.ws JSON body. --dump-dom makes Chrome issue a real HTTPS GET (with
// its real TLS stack) and print the response body to stdout.
func captureWithChrome(bin, endpoint, mode, display string) ([]byte, error) {
	args := []string{
		"--no-sandbox", "--disable-gpu", "--disable-dev-shm-usage",
		"--no-first-run", "--no-default-browser-check",
		"--headless=new", "--dump-dom", endpoint,
	}
	cmd := exec.Command(bin, args...)
	if mode == "headful" {
		if display == "" {
			return nil, fmt.Errorf("headful mode requires -display (run under Xvfb, e.g. -display :99)")
		}
		cmd.Args = []string{"--no-sandbox", "--disable-gpu", "--disable-dev-shm-usage",
			"--no-first-run", "--no-default-browser-check", "--dump-dom", endpoint}
		cmd.Env = append(os.Environ(), "DISPLAY="+display)
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("chrome run: %w (stderr: %s)", err, string(out))
	}
	return out, nil
}
