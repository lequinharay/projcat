package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"projcat/internal/app"
)

func main() {
	// オプションの処理
	filterPtr := flag.String("filter", "", "ファイル名に対する部分一致フィルタリング（カンマ区切りでOR条件）")

	flag.Usage = func() {
		// 全体的な説明（ツールの概要）
		fmt.Fprintf(os.Stderr, "Usage: projcat [OPTIONS] [SUBDIRECTORY]\n\n")
		fmt.Fprintf(os.Stderr, "Description:\n")
		fmt.Fprintf(os.Stderr, "  projcat (pcat) は、プロジェクトの構造、メタドキュメント、およびソースコードを\n")
		fmt.Fprintf(os.Stderr, "  構造化された1ファイルのMarkdownテキストとして美しく連結・出力するCLIツールです。\n\n")

		// 引数の説明
		fmt.Fprintf(os.Stderr, "Arguments:\n")
		fmt.Fprintf(os.Stderr, "  SUBDIRECTORY\n")
		fmt.Fprintf(os.Stderr, "        走査対象とする特定のサブディレクトリ（省略時はGitルート全体を走査）\n\n")

		// オプション（フラグ）の説明
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	var isClipboard bool
	flag.BoolVar(&isClipboard, "clipboard", false, "出力をクリップボードに直接コピーする")
	flag.BoolVar(&isClipboard, "c", false, "出力をクリップボードに直接コピーする（--clipboard の省略形）")

	// パースを実行（これによって、フラグ以外の引数が flag.Args() に切り分けられる）
	flag.Parse()

	// フラグを除いた残りの引数（ターゲットディレクトリ）を取得
	inputDir := ""
	args := flag.Args()
	if len(args) > 0 {
		inputDir = args[0]
	}

	// 引数の意味を解釈して、具体的な絶対パス（文字列）に解決する
	targetDir, err := resolveTargetDir(inputDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "エラー: %v\n", err)
		os.Exit(1)
	}

	// string から canonicalPath へ変換
	cPath, err := app.ToCanonicalPath(targetDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "エラー: パスの変換に失敗しました: %v\n", err)
		os.Exit(1)
	}

	if !cPath.Exists() {
		fmt.Fprintf(os.Stderr, "エラー: 指定のパスが存在しません: %v\n", cPath.String())
	}

	// オプション構造体の組み立て
	opts := &app.RunOptions{
		Filter:    *filterPtr,
		Clipboard: isClipboard,
	}

	// メイン処理を呼ぶ前に環境を検証する
	if err := app.ValidateEnvironment(opts); err != nil {
		fmt.Fprintf(os.Stderr, "エラー: %v\n", err)
		os.Exit(1)
	}

	// オプションを渡して実行
	output, err := app.ScanAndAggregate(cPath, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "エラー: %v\n", err)
		os.Exit(1)
	}

	// 結果を標準出力へ書き出し
	fmt.Print(output)
}

// resolveTargetDir は、ユーザーの入力を評価し、適切なターゲットディレクトリの絶対パスを返します。
// 入力が空文字 "" または "." の場合は、Gitルートディレクトリを自動検出します。
func resolveTargetDir(input string) (string, error) {
	if input != "" && input != "." {
		return input, nil
	}

	// 空文字または "." の場合は Git ルートを探す
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("Gitルートの取得に失敗しました (Gitリポジトリ内で実行してください): %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}
