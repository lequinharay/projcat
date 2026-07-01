package app

import (
	"io/fs"
	"path/filepath"
	"testing"

	"os"
	"os/exec"
)

func init() {
	// テスト実行時は実際の pbcopy を叩かず、常に正常終了させる
	clipboardWriter = func(text string) error {
		return nil
	}
}

// テスト用の一時ディレクトリをGitリポジトリ化した状態で返すユーティリティ関数
func TempGitDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "gitrepo-*")
	if err != nil {
		t.Fatalf("一時ディレクトリの作成に失敗: %v", err)
	}
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("テスト用Gitリポジトリの初期化に失敗: %v", err)
	}
	return dir
}

// 空のテスト用Gitリポジトリを作り、その CanonicalPath を返す
func CreateTestGitRoot(t *testing.T) CanonicalPath {
	t.Helper()
	rootStr := TempGitDir(t)
	root, err := ToCanonicalPath(rootStr)
	if err != nil {
		t.Fatalf("failed to create git root: %v", err)
	}
	return root
}

// 指定された相対パスにファイルを生成し、その CanonicalPath を返す
func WriteTestFile(t *testing.T, targetDir CanonicalPath, relPathStr string, content string) CanonicalPath {
	t.Helper()
	fullPath := filepath.Join(targetDir.String(), relPathStr)

	// 親ディレクトリがなければ自動で作る
	_ = os.MkdirAll(filepath.Dir(fullPath), 0755)

	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file %s: %v", relPathStr, err)
	}

	path, err := ToCanonicalPath(fullPath)
	if err != nil {
		t.Fatalf("failed to canonicalize file path %s: %v", fullPath, err)
	}
	return path
}

// 指定された相対パスにディレクトリを生成し、その CanonicalPath を返す
func MakeTestDir(t *testing.T, targetDir CanonicalPath, relPathStr string) CanonicalPath {
	t.Helper()
	fullPath := filepath.Join(targetDir.String(), relPathStr)
	if err := os.MkdirAll(fullPath, 0755); err != nil {
		t.Fatalf("failed to make test directory %s: %v", relPathStr, err)
	}

	path, err := ToCanonicalPath(fullPath)
	if err != nil {
		t.Fatalf("failed to canonicalize dir path %s: %v", fullPath, err)
	}
	return path
}

// パスから、WalkDirの途中で手に入るような os.DirEntry を一撃で生成する
func GetDirEntry(t *testing.T, path CanonicalPath) os.DirEntry {
	t.Helper()
	fInfo, err := os.Stat(path.String())
	if err != nil {
		t.Fatalf("failed to stat path %s: %v", path.String(), err)
	}
	return fs.FileInfoToDirEntry(fInfo)
}
