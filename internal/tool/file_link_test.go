package tool

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
)

func createDirectoryLink(t *testing.T, target, link string) {
	t.Helper()

	if runtime.GOOS != "windows" {
		if err := os.Symlink(target, link); err != nil {
			t.Fatalf("Symlink() error = %v", err)
		}
		return
	}

	output, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
	if err != nil {
		t.Fatalf("create junction error = %v, output = %s", err, output)
	}
}
