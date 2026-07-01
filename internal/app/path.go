package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CanonicalPath は完全にクリーンアップされた絶対実パスであることを保証する型
type CanonicalPath struct {
	value string
}

// ToCanonicalPath は生文字列を、評価・正規化済みの絶対実パスへと変換する
func ToCanonicalPath(path string) (CanonicalPath, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return CanonicalPath{}, fmt.Errorf("絶対パスへの変換に失敗: %w", err)
	}

	// パスをクリーンにする（実在しない場合のデフォルト）
	cleaned := filepath.Clean(abs)

	// パスが実在するか（またはシンボリックリンクとして存在するか）を確認
	if _, err := os.Lstat(cleaned); err == nil {
		// 実在する場合のみ、シンボリックリンクを厳密に解決する
		eval, err := filepath.EvalSymlinks(cleaned)
		if err != nil {
			return CanonicalPath{}, fmt.Errorf("シンボリックリンクの解決に失敗: %w", err)
		}
		cleaned = eval
	}

	return CanonicalPath{value: cleaned}, nil
}

func (c CanonicalPath) String() string {
	if c.value == "" {
		panic("初期化されていない不正なパス型が使用されました") // あるいは安全なデフォルト値を返す
	}
	return c.value
}

// RelToGitRoot は、絶対パスからGitルート基準の相対パス（GitRelPath）を型安全に計算する
func (c CanonicalPath) RelToGitRoot(gitRoot CanonicalPath) gitRelPath {
	rel, err := filepath.Rel(gitRoot.String(), c.String())
	if err != nil {
		// 万が一計算できない場合は生文字列をフォールバック（型で保護されているため安全）
		return gitRelPath{value: c.String(), canonical: c}
	}
	return gitRelPath{value: rel, canonical: c}
}

// Exists はパスが実際にファイルシステム上に存在するか判定する
func (c CanonicalPath) Exists() bool {
	_, err := os.Stat(c.value)
	return err == nil
}

// IsDotFile は、このパスが指すファイルまたはディレクトリの名前が `.` で始まるか判定する
func (c CanonicalPath) IsDotFile() bool {
	return strings.HasPrefix(filepath.Base(c.value), ".")
}

// Base は、パスの末尾の要素（ファイル名やディレクトリ名）を返す
func (c CanonicalPath) Base() string {
	return filepath.Base(c.value)
}

func (c CanonicalPath) Dir() string {
	return filepath.Dir(c.value)
}

// gitRelPath はGitリポジトリルートからの相対パスであることを保証する型
type gitRelPath struct {
	value     string
	canonical CanonicalPath
}

func (g gitRelPath) String() string {
	if g.value == "" {
		panic("初期化されていない不正なパス型が使用されました") // あるいは安全なデフォルト値を返す
	}
	// mac環境で実行するとfilepath.ToSlash()は何もしてくれないのでテストにならない
	return strings.ReplaceAll(g.value, `\`, "/")
}

// Exists はパスが実際にファイルシステム上に存在するか判定する
// gitRoot を教えてあげることで、初めて本当の場所を os.Stat できる
func (g gitRelPath) Exists() bool {
	return g.canonical.Exists()
}
