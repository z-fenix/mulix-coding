package quality

import (
	"archive/zip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/z-fenix/mulix-coding/internal/fileutil"
)

const maxArtifactSize = 256 << 20
const mavenKarateMetadata = "META-INF/maven/io.karatelabs.ext/karate-agent/pom.properties"

type karateTool struct {
	Java, Jar, Digest string
	ProjectEnv        []string
}

// Open a stable ordinary input, with a limit before doing any hashing/copying.
func openArtifact(name string, limit int64) (*os.File, int64, error) {
	info, err := os.Lstat(name)
	if err != nil {
		return nil, 0, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > limit {
		return nil, 0, fmt.Errorf("expected nonempty regular input up to %d bytes: %s", limit, name)
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, 0, err
	}
	actual, err := f.Stat()
	if err != nil || !os.SameFile(info, actual) || actual.Size() != info.Size() {
		_ = f.Close()
		return nil, 0, fmt.Errorf("input changed while opening: %s", name)
	}
	return f, info.Size(), nil
}

func checkedDigest(r io.Reader, size int64) (string, error) {
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(r, size+1))
	if err != nil {
		return "", err
	}
	if n != size {
		return "", fmt.Errorf("input length changed while reading")
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func jarVersion(f *os.File, size int64) (string, error) {
	z, err := zip.NewReader(f, size)
	if err != nil {
		return "", err
	}
	b, err := z.Open(mavenKarateMetadata)
	if err != nil {
		return "", err
	}
	defer b.Close()
	data, err := io.ReadAll(io.LimitReader(b, 4097))
	if err != nil || len(data) > 4096 {
		return "", fmt.Errorf("invalid Karate Maven metadata")
	}
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "version="); ok {
			return v, nil
		}
	}
	return "", fmt.Errorf("Karate Maven metadata does not contain a version")
}

func inspectKarate(s Suite) (karateTool, error) {
	var tool karateTool
	jar := os.Getenv("MULIX_KARATE_JAR")
	if !filepath.IsAbs(jar) {
		return tool, fmt.Errorf("set MULIX_KARATE_JAR to the absolute path of the pinned Karate JAR")
	}
	f, size, err := openArtifact(filepath.Clean(jar), maxArtifactSize)
	if err != nil {
		return tool, fmt.Errorf("open Karate JAR: %w", err)
	}
	defer f.Close()
	jarHash, err := checkedDigest(f, size)
	if err != nil || jarHash != s.Karate.SHA256 {
		return tool, fmt.Errorf("Karate JAR SHA-256 does not match the approved policy")
	}
	version, err := jarVersion(f, size)
	if err != nil || version != s.Karate.Version {
		return tool, fmt.Errorf("Karate JAR is not the approved version %s: %v", s.Karate.Version, err)
	}
	java := os.Getenv("MULIX_KARATE_JAVA")
	if java == "" {
		java, err = exec.LookPath("java")
		if err != nil {
			return tool, fmt.Errorf("Karate requires a Java 21+ installation: %w", err)
		}
	}
	if !filepath.IsAbs(java) {
		return tool, fmt.Errorf("MULIX_KARATE_JAVA must name an absolute Java executable path")
	}
	java, err = filepath.EvalSymlinks(java)
	if err != nil {
		return tool, fmt.Errorf("resolve Java executable: %w", err)
	}
	binary, binarySize, err := openArtifact(java, maxArtifactSize)
	if err != nil {
		return tool, fmt.Errorf("open Java executable: %w", err)
	}
	defer binary.Close()
	binaryHash, err := checkedDigest(binary, binarySize)
	if err != nil {
		return tool, err
	}
	releaseFile, _, err := openArtifact(filepath.Join(filepath.Dir(filepath.Dir(java)), "release"), 32768)
	if err != nil {
		return tool, fmt.Errorf("read Java release metadata (use a Java 21+ installation's bin/java): %w", err)
	}
	defer releaseFile.Close()
	release, err := io.ReadAll(io.LimitReader(releaseFile, 32769))
	if err != nil || len(release) > 32768 {
		return tool, fmt.Errorf("invalid Java release metadata")
	}
	major := 0
	for _, line := range strings.Split(string(release), "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), "JAVA_VERSION="); ok {
			value = strings.Trim(value, `"`)
			versions := strings.FieldsFunc(value, func(r rune) bool { return r == '.' || r == '-' || r == '+' })
			if len(versions) > 0 {
				major, _ = strconv.Atoi(versions[0])
			}
			break
		}
	}
	if major < 21 {
		return tool, fmt.Errorf("Karate requires Java 21 or newer")
	}
	for _, name := range s.Karate.Env {
		if value, ok := os.LookupEnv(name); ok {
			tool.ProjectEnv = append(tool.ProjectEnv, name+"="+value)
		}
	}
	slices.Sort(tool.ProjectEnv)
	tool.Java, tool.Jar = java, filepath.Clean(jar)
	tool.Digest = Digest([]byte(strings.Join([]string{jarHash, java, binaryHash, Digest(release), s.Karate.Environment, strings.Join(tool.ProjectEnv, "\x00")}, "\x00")))
	return tool, nil
}

// ExecutionFingerprint additionally binds the actual Java/JAR and explicit
// environment inputs for Karate. It opens files only, never starts Java, so
// transition and receipt-only checks cannot run another suite.
func ExecutionFingerprint(p Project, c Contract, task int) (Snapshot, error) {
	snap, err := Fingerprint(p, c, task)
	if err != nil {
		return snap, err
	}
	policy, err := executionPolicy(p, c)
	if err != nil {
		return snap, err
	}
	for _, suite := range policy.Suites {
		if suite.Format == KarateFormat && suiteSelected(c, suite.ID, task) {
			tool, err := inspectKarate(suite)
			if err != nil {
				return snap, err
			}
			snap.Karate = tool.Digest
		}
	}
	return snap, nil
}

type karateInvocation struct {
	Argv          []string
	Env           []string
	Dir           string
	RunDir        string
	Features      []string
	Identity      string
	KarateSources string `json:"karate_sources"`
}

func prepareKarate(p Project, c Contract, s Suite, task int, scratch, runID string) (karateInvocation, error) {
	var inv karateInvocation
	tool, err := inspectKarate(s)
	if err != nil {
		return inv, err
	}
	selection, err := karateInputs(p, c, s, task)
	if err != nil {
		return inv, err
	}
	inv.RunDir = filepath.Join(scratch, runID+"-"+s.ID+"-karate")
	if err := os.Mkdir(inv.RunDir, 0700); err != nil {
		return inv, err
	}
	inv.Dir = filepath.Join(inv.RunDir, "inputs")
	home := filepath.Join(inv.RunDir, "home")
	temp := filepath.Join(inv.RunDir, "temp")
	for _, dir := range []string{filepath.Join(inv.Dir, "helpers"), home, temp} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return inv, err
		}
	}
	stagedJar := filepath.Join(inv.RunDir, "engine.jar")
	original, size, err := openArtifact(tool.Jar, maxArtifactSize)
	if err != nil {
		return inv, err
	}
	defer original.Close()
	copy, err := os.OpenFile(stagedJar, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return inv, err
	}
	digest, copyErr := checkedDigest(io.TeeReader(original, copy), size)
	closeErr := copy.Close()
	if copyErr != nil || closeErr != nil || digest != s.Karate.SHA256 {
		return inv, fmt.Errorf("could not stage the exact approved Karate JAR")
	}
	prefix := runtimeTestsRoot(p) + "/"
	staged := sha256.New()
	files := append(slices.Clone(selection.Features), selection.Helpers...)
	slices.Sort(files)
	for _, rel := range files {
		source, err := fileutil.ControlPath(p.Root, rel, false)
		if err != nil {
			return inv, err
		}
		target := filepath.Join(inv.Dir, filepath.FromSlash(strings.TrimPrefix(rel, prefix)))
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return inv, err
		}
		f, _, err := openArtifact(source, outputLimit)
		if err != nil {
			return inv, err
		}
		data, err := io.ReadAll(io.LimitReader(f, outputLimit+1))
		_ = f.Close()
		if err != nil || len(data) > outputLimit {
			return inv, fmt.Errorf("invalid Karate test source: %s", rel)
		}
		if err := os.WriteFile(target, data, 0600); err != nil {
			return inv, err
		}
		stampKarateSource(staged, rel, data)
	}
	inv.KarateSources = fmt.Sprintf("%x", staged.Sum(nil))
	output := filepath.Join(inv.RunDir, "output")
	inv.Argv = []string{tool.Java, "-Duser.home=" + home, "-Djava.io.tmpdir=" + temp, "-Dfile.encoding=UTF-8", "-jar", stagedJar, "run", "--no-pom", "-w", inv.Dir, "-g", filepath.Join(inv.Dir, "helpers"), "-f", "~html,karate:jsonl", "-o", output}
	if s.Karate.Environment != "" {
		inv.Argv = append(inv.Argv, "-e", s.Karate.Environment)
	}
	for _, rel := range selection.Features {
		inv.Argv = append(inv.Argv, filepath.Join(inv.Dir, filepath.FromSlash(strings.TrimPrefix(rel, prefix))))
	}
	inv.Env = append(slices.Clone(tool.ProjectEnv), "HOME="+home, "USERPROFILE="+home, "TMPDIR="+temp, "TEMP="+temp, "TMP="+temp, "KARATE_LOG_FILE="+filepath.Join(inv.RunDir, "karate-agent.log"))
	for _, key := range []string{"SystemRoot", "WINDIR"} {
		if value, ok := os.LookupEnv(key); ok {
			inv.Env = append(inv.Env, key+"="+value)
		}
	}
	inv.Features = selection.Features
	inv.Identity = tool.Digest
	return inv, nil
}

func karateEventPath(inv karateInvocation) (string, error) {
	return fileutil.ControlPath(inv.RunDir, path.Join("output", "karate-json", "karate-events.jsonl"), false)
}
