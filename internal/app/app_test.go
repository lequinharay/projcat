package app

import (
	"testing"

	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func TestValidateTargetDir(t *testing.T) {
	t.Run("存在しないディレクトリはエラーになること", func(t *testing.T) {
		err := ValidateTargetDir("/path/to/invalid/dir")
		if err == nil {
			t.Errorf("期待値: エラーが発生すること, 実際の値: nil")
		}
	})

	t.Run("有効なGitリポジトリ（カレント）は正常終了すること", func(t *testing.T) {
		err := ValidateTargetDir(".")
		if err != nil {
			t.Errorf("期待値: エラーなし, 実際の値: %v", err)
		}
	})
}
func TestToCanonicalPath_NonExistent(t *testing.T) {
	t.Run("存在しないパスを指定してもエラーにならずCanonicalPathが生成され、Existsがfalseを返すこと", func(t *testing.T) {
		// 絶対に存在しない一時パスを指定
		nonExistentPath := "/path/to/never/existed/file/or/dir/9999"

		// 現行の実装だと、ここで EvalSymlinks がエラーを返すため Red（失敗）になる
		p, err := ToCanonicalPath(nonExistentPath)
		if err != nil {
			t.Fatalf("期待値: エラーなしで型を生成できること, 実際の値: %v", err)
		}

		// 生成されたオブジェクトの Exists() が正しく false を返すか検証
		if p.Exists() {
			t.Errorf("期待値: Exists() が false であること, 実際の値: true")
		}
	})
}

func TestParseArgs(t *testing.T) {
	t.Run("引数が空の場合はカレントディレクトリを返すこと", func(t *testing.T) {
		args := []string{}
		expected := "."

		actual := ParseArgs(args)
		if actual != expected {
			t.Errorf("期待値: %s, 実際の値: %s", expected, actual)
		}
	})

	t.Run("引数が1つある場合はその値を返すこと", func(t *testing.T) {
		args := []string{"/path/to/target"}
		expected := "/path/to/target"

		actual := ParseArgs(args)
		if actual != expected {
			t.Errorf("期待値: %s, 実際の値: %s", expected, actual)
		}
	})
}

func TestDetermineFence(t *testing.T) {
	t.Run("中身にバックティックが含まれない場合は4つを返すこと", func(t *testing.T) {
		content := "func main() {}"
		expected := "````"

		actual := determineFence(content)
		if actual != expected {
			t.Errorf("期待値: %s, 実際の値: %s", expected, actual)
		}
	})

	t.Run("中身に3つの連続バックティックがある場合は4つを返すこと", func(t *testing.T) {
		content := "```go\nfmt.Println()\n```"
		expected := "````"

		actual := determineFence(content)
		if actual != expected {
			t.Errorf("期待値: %s, 実際の値: %s", expected, actual)
		}
	})

	t.Run("中身に4つの連続バックティックがある場合は5つを返すこと", func(t *testing.T) {
		content := "````\nなにかのネスト\n````"
		expected := "`````"

		actual := determineFence(content)
		if actual != expected {
			t.Errorf("期待値: %s, 実際の値: %s", expected, actual)
		}
	})
}

func TestIsIgnoredByGit(t *testing.T) {
	t.Run("shouldIgnoreDirディレクトリ内のパスは無視されること", func(t *testing.T) {
		gitRoot := CreateTestGitRoot(t)

		// .gitignoreに無視ディレクトリを登録
		WriteTestFile(t, gitRoot, ".gitignore", "shouldIgnoreDir\n")

		// 無視ディレクトリを作成
		shouldIgnoreDir := MakeTestDir(t, gitRoot, "shouldIgnoreDir")
		relPath := shouldIgnoreDir.RelToGitRoot(gitRoot)

		if !isIgnoredByGit(relPath, gitRoot, true) {
			t.Errorf("期待値: true (無視される), 実際の値: false")
		}
	})

	t.Run("末尾スラッシュ付きパターン（shouldIgnoreDir/）に対して、ディレクトリとしてのパスが確実に無視判定されること", func(t *testing.T) {
		gitRoot := CreateTestGitRoot(t)

		// .gitignore に末尾スラッシュ付きで登録
		WriteTestFile(t, gitRoot, ".gitignore", "shouldIgnoreDir/\n")

		// 無視されるべき階層化ディレクトリ
		shouldIgnoreDir := MakeTestDir(t, gitRoot, "shouldIgnoreDir")
		shouldIgnoreDirChild := MakeTestDir(t, shouldIgnoreDir, "shouldIgnoreDirChild")
		relPath := shouldIgnoreDirChild.RelToGitRoot(gitRoot)

		// ディレクトリ判定 (true) で呼び出し
		if !isIgnoredByGit(relPath, gitRoot, true) {
			t.Errorf("期待値: true (上位の shouldIgnoreDir/ によって無視されるべき), 実際の値: false")
		}
	})

	t.Run("main.goなどの通常のファイルは無視されないこと", func(t *testing.T) {
		gitRoot := CreateTestGitRoot(t)
		mainPath := WriteTestFile(t, gitRoot, "main.go", "package main")

		relPath := mainPath.RelToGitRoot(gitRoot)
		if isIgnoredByGit(relPath, gitRoot, false) {
			t.Errorf("期待値: false (無視されない), 実際の値: true")
		}
	})

	t.Run("中間ディレクトリにファイルがなく、ディレクトリのみが入れ子になっている構造（dir1/dir2/file.txt）が正しくツリーに出力されること", func(t *testing.T) {
		gitRoot := CreateTestGitRoot(t)
		nestedDir := MakeTestDir(t, gitRoot, "dir1/dir2")
		WriteTestFile(t, nestedDir, "files.txt", "hello")

		// 走査を実行
		result, err := ScanAndAggregate(gitRoot, nil)
		if err != nil {
			t.Fatalf("予期せぬエラー: %v", err)
		}

		// ## DIR セクション内のツリー形状を検証
		if !strings.Contains(result, "## DIR\n") {
			t.Fatalf("## DIR セクションが出力に含まれていません")
		}

		// 中間層 dir1、その配下の dir2、そして files.txt が正しくインデントされて描画されているかを期待
		expectedTree := "└── dir1\n" +
			"    └── dir2\n" +
			"        └── files.txt"

		if !strings.Contains(result, expectedTree) {
			t.Errorf("期待するツリー構造が正しくレンダリングされていません。\n【期待値】:\n%s\n\n【実際の出力】:\n%s", expectedTree, result)
		}
	})

	t.Run(".projcatignore の単一記述はファイル名として扱い、ワイルドカードがない限り同名ディレクトリ配下は無視しないこと", func(t *testing.T) {
		gitRoot := CreateTestGitRoot(t)

		// .projcatignore には「projcat」とだけ記述（ファイル扱い）
		ignoreFile := WriteTestFile(t, gitRoot, ".projcatignore", "projcat\n")

		// ルート直下のファイル「projcat」（バイナリ等）を作成
		WriteTestFile(t, gitRoot, "projcat", "binary mock")

		// 同名のディレクトリ「cmd/projcat」の中に、無視されてはならない「main.go」を配置
		visibleDir := MakeTestDir(t, gitRoot, "cmd/projcat")
		WriteTestFile(t, visibleDir, "main.go", "package main")

		// 4. 走査を実行
		result, err := ScanAndAggregate(gitRoot, nil)
		if err != nil {
			t.Fatalf("予期せぬエラー: %v", err)
		}

		// 検証①: ルート直下のファイル「projcat」はちゃんと無視されていること
		if strings.Contains(result, "```"+ignoreFile.Base()+"\n") {
			t.Errorf("無視対象であるはずのファイル 'projcat' が FILES セクションに含まれています")
		}

		// 検証②: 「cmd/projcat/main.go」は巻き添えにされず、DIR と FILES に含まれていること
		if !strings.Contains(result, "cmd") || !strings.Contains(result, "projcat") || !strings.Contains(result, "main.go") {
			t.Errorf("中間ディレクトリ 'cmd/projcat' 配下が消失しています。\n実際の出力:\n%s", result)
		}
	})
}

func TestLoadProjcatIgnorer(t *testing.T) {
	t.Run(".projcatignoreファイルが存在しない場合はnilを返すこと", func(t *testing.T) {
		gitRoot := CreateTestGitRoot(t)

		ignorer, err := loadProjcatIgnore(gitRoot)
		if err != nil {
			t.Fatalf("予期せぬエラー: %v", err)
		}
		if ignorer != nil {
			t.Errorf("期待値: nil, 実際の値: %v", ignorer)
		}
	})

	t.Run(".projcatignoreファイルを正しくパースしてマッチングできること", func(t *testing.T) {
		gitRoot := CreateTestGitRoot(t)

		// ダミーの無視ファイルを作成
		WriteTestFile(t, gitRoot, ".projcatignore", "*.log\nprojcat")

		patterns, err := loadProjcatIgnore(gitRoot) // 読み込み関数名や引数は実装に合わせて調整してください
		if err != nil {
			t.Fatalf("予期せぬエラー: %v", err)
		}
		// 期待されるパターンが正しくスライスに含まれているか検証
		expectedPatterns := map[string]bool{
			"*.log":   true,
			"projcat": true,
		}

		if len(patterns) != len(expectedPatterns) {
			t.Errorf("期待されるパターン数: %d, 実際の値: %d (取得内容: %v)", len(expectedPatterns), len(patterns), patterns)
		}

		for _, p := range patterns {
			if !expectedPatterns[p] {
				t.Errorf("予期せぬパターンがパースされています: %s", p)
			}
		}
	})

	t.Run(".projcatpromptfiles の指定に基づき、ファイルおよびディレクトリ配下の全ファイルが PROMPT セクションに正しく抽出されること", func(t *testing.T) {
		gitRoot := CreateTestGitRoot(t)

		// .projcatpromptfiles を作成
		// ルート直下の単一ファイル「explicit.md」と、ディレクトリ配下すべてを指定する「prompts/*」を記述
		WriteTestFile(t, gitRoot, ".projcatpromptfiles", "explicit.md\nprompts/*\n")

		// .projcatignore を作成（prompts/* の中にある特定のファイルを無視設定する）
		WriteTestFile(t, gitRoot, ".projcatignore", "prompts/sub/ignored.txt\n")

		/**
		 * 対象となるファイル群を配置
		 * 以上のsetUpで作られるテスト環境のディレクトリ構造とファイル配置:
		 * .
		 * ├── .projcatpromptfiles   <-- [内容] "explicit.md\nprompts/*\n"
		 * ├── explicit.md           <-- [期待] PROMPTに対象として含まれる（ファイル直接指定）
		 * ├── .clinerules           <-- [期待] 無視される（リストに未記載の隠しファイル）
		 * ├── unlisted.md           <-- [期待] 無視される（リストに未記載の通常ファイル）
		 * └── prompts/
		 *     └── sub/
		 *          ├── rules.txt    <-- [期待] PROMPTに対象として含まれる（ディレクトリ再帰指定）
		 *          └── ignored.txt  <-- [期待] 無視される（.projcatignore で弾かれているファイル）
		 */
		// ホワイトリストに明示されたルートファイル
		WriteTestFile(t, gitRoot, "explicit.md", "Explicit Prompt Content")

		// prompts/* によって再帰的に拾われるべき深い階層のファイル群
		nestedDir := MakeTestDir(t, gitRoot, "prompts/sub")
		WriteTestFile(t, nestedDir, "rules.txt", "Nested Prompt Content")

		// prompts/* の配下にあるが、.projcatignore で明示的に無視されているファイル
		WriteTestFile(t, gitRoot, "ignored.txt", "Ignored Prompt Content")

		// ホワイトリストに未記載の通常ファイル
		WriteTestFile(t, gitRoot, "unlisted.md", "unlisted Content")

		// リストに含まれていない無視されるべき「隠しファイル（.clinerules）」を配置
		WriteTestFile(t, gitRoot, ".clinerules", "ignored dot file content")

		/* 走査を実行 */
		result, err := ScanAndAggregate(gitRoot, nil)
		if err != nil {
			t.Fatalf("予期せぬエラー: %v", err)
		}

		/* 検証 */
		promptIdx := strings.Index(result, "## PROMPT")
		if promptIdx == -1 {
			t.Fatalf("## PROMPT セクションが出力に含まれていません, 実際の出力:\n%s", result)
		}
		filesIdx := strings.Index(result, "## FILES\n")
		if filesIdx == -1 {
			t.Fatalf("## FILES セクションが出力に含まれていません, 実際の出力:\n%s", result)
		}

		// PROMPT セクションだけのテキストをスライスしてスコープを限定する
		promptSectionText := result[promptIdx:filesIdx]

		// 【ポジティブ検証】
		if !strings.Contains(promptSectionText, "```explicit.md\nExplicit Prompt Content") {
			t.Errorf("explicit.md の内容が PROMPT セクションに正しく含まれていません, 実際の出力:\n%s", result)
		}
		if !strings.Contains(promptSectionText, "```prompts/sub/rules.txt\nNested Prompt Content") {
			t.Errorf("prompts/* 指定による配下のファイル rules.txt が PROMPT セクションに含まれていません, 実際の出力:\n%s", result)
		}

		// 【ネガティブ検証：含まれるべきでないファイル】
		if strings.Contains(promptSectionText, "unlisted.md") {
			t.Errorf(".projcatpromptfiles に未記載の通常ファイル 'unlisted.md' が PROMPT セクションに含まれてしまっています, 実際の出力:\n%s", result)
		}
		if strings.Contains(promptSectionText, ".clinerules") {
			t.Errorf(".projcatpromptfiles に未記載の隠しファイル '.clinerules' が PROMPT セクションに含まれてしまっています, 実際の出力:\n%s", result)
		}
		if strings.Contains(promptSectionText, "ignored.txt") {
			t.Errorf(".projcatignore で無視指定されている 'ignored.txt' が PROMPT セクションに含まれてしまっています, 実際の出力:\n%s", result)
		}
	})
}

func TestGetGitRoot(t *testing.T) {
	t.Run("Gitリポジトリ内のサブディレクトリからでも正しいルートディレクトリを取得できること", func(t *testing.T) {
		gitRoot := CreateTestGitRoot(t)

		// 深いサブディレクトリを作成
		subDir := MakeTestDir(t, gitRoot, "deep/nested/dir")

		// サブディレクトリのパスを引数にして GetGitRoot を実行
		subGitRoot, err := getGitRoot(subDir)
		if err != nil {
			t.Fatalf("GetGitRootでエラーが発生しました: %v", err)
		}

		if subGitRoot.String() != gitRoot.String() {
			t.Errorf("期待値:\n    %s,\n実際の値:\n    %s", gitRoot.String(), subGitRoot.String())
		}
	})
}

func TestGetGitRoot_NonExistent(t *testing.T) {
	t.Run("存在しないパスをgetGitRootに渡した場合に、適切なエラーとゼロ値が返ること", func(t *testing.T) {
		// 存在しないパスで CanonicalPath を作成（新仕様によりエラーにならない）
		nonExistentPath, err := ToCanonicalPath("/path/to/never/existed/dir/12345")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// getGitRoot を呼び出す
		root, err := getGitRoot(nonExistentPath)

		// エラーが返ってくることを検証
		if err == nil {
			t.Errorf("期待値: エラーが発生すること, 実際の値: nil")
		}

		// 戻り値がゼロ値（空の構造体）であることを検証
		if root != (CanonicalPath{}) {
			t.Errorf("期待値: ゼロ値のCanonicalPath, 実際の値: %v", root)
		}
	})
}

func TestGitRelPath_ToSlash(t *testing.T) {
	t.Run("Windowsスタイルのバックスラッシュを含むパスであっても常にスラッシュ区切りに統一されること", func(t *testing.T) {
		gitRoot := CreateTestGitRoot(t)

		// テスト用に確実に実在するファイル（例: main.go）への絶対パスを正しく作る
		realFile := filepath.Join(gitRoot.String(), "main.go")
		cPath, err := ToCanonicalPath(realFile)
		if err != nil {
			t.Fatalf("対象文字列をCanonical化できなかった: %v", err)
		}

		// 正しい生成ロジックで一度 gitRelPath を生成する
		rel := cPath.RelToGitRoot(gitRoot)

		pathStr := `cmd\projcat\main.go`
		// 内部の value を Windows 風のバックスラッシュを含むパスに「意図的に汚染」させる
		rel.value = pathStr

		expected := "cmd/projcat/main.go"

		// String() メソッドが返す値がスラッシュ区切りになっているか
		if rel.String() != expected {
			t.Errorf("期待値: %s, 実際の値: %s", expected, rel.String())
		}
	})
}

func TestScanAndAggregate(t *testing.T) {
	t.Run("複数階層のディレクトリ内のファイルを走査して正しい相対パスで結合すること", func(t *testing.T) {
		gitRoot := CreateTestGitRoot(t)

		// 1階層目のファイル
		WriteTestFile(t, gitRoot, "root.go", "package root")

		// 2階層目（深いディレクトリ）を作成しファイルを置く
		deepDir := MakeTestDir(t, gitRoot, "src/utils")
		WriteTestFile(t, deepDir, "math.go", "package utils")

		/* 走査を実行 */
		result, err := ScanAndAggregate(gitRoot, nil)
		if err != nil {
			t.Fatalf("予期せぬエラー: %v, gitRoot: %s", err, gitRoot)
		}

		// 1階層目のファイルが正しく含まれているか
		expectedRootHeader := "```root.go"
		if !strings.Contains(result, expectedRootHeader) {
			t.Errorf("期待値: ルートファイル %q が含まれること, 実際の出力:\n%s", expectedRootHeader, result)
		}

		// 複数階層のファイルが「正しい相対パス（スラッシュ区切り）」で含まれているか
		// @todo Windows環境でもスラッシュ区切りで出力されるかどうかを確認する必要がある
		expectedDeepHeader := "```src/utils/math.go"

		// もし Windows 環境（\）でもスラッシュ（/）で統一して出力したい仕様であれば、
		// 現在の実装がそれを満たしているかどうかが関係する
		if !strings.Contains(result, expectedDeepHeader) {
			t.Errorf("期待値: %q が含まれること, 実際の出力:\n%s", expectedDeepHeader, result)
		}
	})

	t.Run(".projcatignoreに記載されたファイルが正しく除外されること", func(t *testing.T) {
		gitRoot := CreateTestGitRoot(t)

		// 非除外ファイルと除外ファイルを準備
		WriteTestFile(t, gitRoot, "keep.go", "pakcage keep")
		WriteTestFile(t, gitRoot, "secret.key", "private-key-data")
		// ignoreファイルを準備
		WriteTestFile(t, gitRoot, ".projcatignore", "secret.key\n")

		/* 走査を実行 */
		result, err := ScanAndAggregate(gitRoot, nil)
		if err != nil {
			t.Fatalf("予期せぬエラー: %v", err)
		}

		if !strings.Contains(result, "keep.go") {
			t.Errorf("keep.go が含まれていません, 実際の出力:\n%s", result)
		}
		if strings.Contains(result, "secret.key") {
			t.Errorf(".projcatignore で指定された secret.key が除外されていません, 実際の出力:\n%s", result)
		}
		if strings.Contains(result, ".projcatignore") {
			t.Errorf(".projcatignore 自体が出力に含まれてしまっています, 実際の出力:\n%s", result)
		}
	})

	t.Run("提示されたセクション構造（#プロジェクト名、## README、## SPEC、## ROADMAP、## PROMPT、## FILES）の通りに出力されること", func(t *testing.T) {
		gitRoot := CreateTestGitRoot(t)
		projectName := estimateProjectName(gitRoot)

		// 各種ファイルを配置（順序をシャッフル）
		WriteTestFile(t, gitRoot, "abc.go", "package abc")
		WriteTestFile(t, gitRoot, "ROADMAP.md", "# RoadmapContent")
		WriteTestFile(t, gitRoot, "README.md", "# READMEContent")
		WriteTestFile(t, gitRoot, "SPEC.md", "# SPECContent")

		// PROMPTセクションは固定値じゃないので特別に準備
		WriteTestFile(t, gitRoot, ".projcatpromptfiles", "prompt.md\n")
		WriteTestFile(t, gitRoot, "prompt.md", "prompt content")

		// 走査を実行
		result, err := ScanAndAggregate(gitRoot, nil)
		if err != nil {
			t.Fatalf("予期せぬエラー: %v", err)
		}

		// セクション構造の文字列一致・出現順序を厳密に検証
		expectedHeader := "# " + projectName + "\n"
		if !strings.HasPrefix(result, expectedHeader) {
			t.Errorf("出力がプロジェクト名のH1ヘッダーから始まっていません。実際の先頭:\n%s", result)
		}

		// 各セクションが正しいフォーマットで配置されているか
		idxREADME := strings.Index(result, "## README\n````README.md\n# READMEContent")
		idxSPEC := strings.Index(result, "## SPEC\n````SPEC.md\n# SPECContent")
		idxROADMAP := strings.Index(result, "## ROADMAP\n````ROADMAP.md\n# RoadmapContent")
		idxPROMPT := strings.Index(result, "## PROMPT\n````prompt.md\nprompt content")
		idxFILES := strings.Index(result, "## FILES\n")

		if idxREADME == -1 || idxSPEC == -1 || idxROADMAP == -1 || idxPROMPT == -1 || idxFILES == -1 {
			t.Fatalf("必要なセクションが正しいフォーマットで含まれていません, 実際の出力:\n%s", result)
		}

		// 順序チェック
		if !(idxREADME < idxSPEC && idxSPEC < idxROADMAP && idxROADMAP < idxPROMPT && idxPROMPT < idxFILES) {
			t.Errorf("セクションの順序が正しくありません, 実際の出力:\n%s", result)
		}

		// 通常ファイルがFILESセクションに格納されているか
		if !strings.Contains(result, "```abc.go\npackage abc") {
			t.Errorf("FILESセクションに abc.go が含まれていません, 実際の出力:\n%s", result)
		}

		// 特別なファイルがFILESセクション以下に重複して露出していないか
		if strings.Count(result, "# READMEContent") != 1 {
			t.Errorf("README の中身が重複して出力されています, 実際の出力:\n%s", result)
		}
	})

	t.Run("## DIRセクションに、無視対象を除外した正しいツリー構造が出力されること", func(t *testing.T) {
		gitRoot := CreateTestGitRoot(t)

		// 通常ファイルと階層構造を作成
		nestedDir1 := MakeTestDir(t, gitRoot, "src")
		nestedDir2 := MakeTestDir(t, nestedDir1, "utils")
		WriteTestFile(t, gitRoot, "main.go", "main")
		WriteTestFile(t, nestedDir1, "a.go", "a")
		WriteTestFile(t, nestedDir2, "b.go", "b")

		// 無視されるべきファイルを配置
		WriteTestFile(t, gitRoot, "secret.key", "secret")
		WriteTestFile(t, gitRoot, ".projcatignore", "secret.key")

		/* 走査を実行 */
		result, err := ScanAndAggregate(gitRoot, nil)
		if err != nil {
			t.Fatalf("予期せぬエラー: %v", err)
		}

		// ## DIR セクションの存在と中身のツリー形状を検証
		if !strings.Contains(result, "## DIR\n") {
			t.Fatalf("## DIR セクションが出力に含まれていません")
		}

		// 期待されるツリーの形状（インデントと罫線）
		// ※ 出力例にあった通り、バックティックフェンスの中にツリーが描画されることを期待
		expectedTree := "├── main.go\n" +
			"└── src\n" +
			"    ├── a.go\n" +
			"    └── utils\n" +
			"        └── b.go"

		if !strings.Contains(result, expectedTree) {
			t.Errorf("期待するツリー構造が含まれていません。\n【期待値】:\n%s\n\n【実際の出力】:\n%s", expectedTree, result)
		}

		// 無視されたファイルがツリーに含まれていないこと
		if strings.Contains(result, "secret.key") {
			t.Errorf("無視対象の secret.key がツリーに出力されてしまっています")
		}
	})

	t.Run("引数にサブディレクトリが指定された場合、特別なファイルとツリーはルートから、FILESは指定ディレクトリ以下のみを走査すること", func(t *testing.T) {
		gitRoot := CreateTestGitRoot(t)

		// ルート直下に特別なファイルと、一般ファイル、別ディレクトリ（cmd）を配置
		WriteTestFile(t, gitRoot, "README.md", "root readme")
		WriteTestFile(t, gitRoot, "main.go", "package main")

		cmdDir := MakeTestDir(t, gitRoot, "cmd")
		WriteTestFile(t, cmdDir, "main.go", "package cmd")

		// 本命の対象サブディレクトリ（src）を作成し、その中に一般ファイルを配置
		subDir := MakeTestDir(t, gitRoot, "src")
		WriteTestFile(t, subDir, "app.go", "package src") // FILESに入るべきファイル

		/**
		 * 生成されるディレクトリ構造：
		 * gitRoot (Gitリポジトリルート)
		 * ├── README.md
		 * ├── main.go
		 * ├── cmd/
		 * │   └── main.go
		 * └── src/
		 *     └── app.go
		 */

		// 引数としてサブディレクトリ（src）を指定して実行
		result, err := ScanAndAggregate(subDir, nil)
		if err != nil {
			t.Fatalf("予期せぬエラー: %v", err)
		}

		expectedProjectName := gitRoot.Base()
		expectedHeader := "# " + expectedProjectName + "\n"
		if !strings.HasPrefix(result, expectedHeader) {
			t.Errorf("期待値: 先頭がリポジトリルート名 %q であること, 実際の出力:\n%s", expectedHeader, result)
		}

		// 特別なファイル(README.md)がルートから取得されていること
		if !strings.Contains(result, "## README\n") || !strings.Contains(result, "root readme") {
			t.Errorf("特別なファイル(README.md)がルートから取得されていません")
		}

		// ツリー構造（DIR）に、同列の「cmd」やルートの「main.go」が含まれていること
		if !strings.Contains(result, "## DIR\n") {
			t.Fatalf("## DIR セクションが出力に含まれていません")
		}
		if !strings.Contains(result, "cmd") {
			t.Errorf("ツリー構造(DIR)に、指定外の同列ディレクトリ 'cmd' が含まれていません。実際の出力:\n%s", result)
		}
		if !strings.Contains(result, "main.go") {
			t.Errorf("ツリー構造(DIR)に、ルート直下の 'main.go' が含まれていません。")
		}

		// FILESセクションには、指定された subDir (src) 以下のファイルのみが含まれていること
		filesIdx := strings.Index(result, "## FILES")
		if filesIdx == -1 {
			t.Fatalf("## FILES セクションが出力に含まれていません")
		}

		// src/app.go はFILESに含まれるべき
		if !strings.Contains(result[filesIdx:], "```src/app.go") {
			t.Errorf("FILESセクションに指定ディレクトリ配下のファイル(src/app.go)が含まれていません")
		}

		// ルート直下の main.go や cmd/main.go が FILES に混ざらない
		if strings.Contains(result[filesIdx:], "```main.go") {
			t.Errorf("FILESセクションに対象外であるルートのファイル(main.go)が混入しています。実際の出力:\n%s", result)
		}
		if strings.Contains(result[filesIdx:], "```cmd/main.go") {
			t.Errorf("FILESセクションに対象外である別ディレクトリのファイル(cmd/main.go)が混入しています。")
		}
	})
}

func TestScanAndAggregate_MissingSpecialSections(t *testing.T) {
	t.Run("README.mdなどのメタドキュメントが存在しないクリーンなディレクトリでも、エラーにならず走査が完了すること", func(t *testing.T) {
		gitRoot := CreateTestGitRoot(t)

		// README.md などを一切作成しない状態で ScanAndAggregate を実行
		_, err := ScanAndAggregate(gitRoot, nil)

		if err != nil {
			t.Fatalf("メタドキュメントが存在しない場合でも正常に動作すべき。エラー: %v", err)
		}
	})
}

func TestShouldSkipEntry_DotFiles(t *testing.T) {
	t.Run("通常のドットファイルはスキップされること", func(t *testing.T) {
		// プロジェクトルート
		gitRoot := CreateTestGitRoot(t)

		// ルートに隠しファイルを配置
		envPath := WriteTestFile(t, gitRoot, ".env", "A=B")

		// walkDir中のカレントディレクトリ
		d := GetDirEntry(t, envPath)

		// ignore: 空, prompt: 空
		skip, err := shouldSkipEntry(envPath, d, gitRoot, []string{}, []string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !skip {
			t.Errorf("通常のドットファイル .env はスキップされるべきです")
		}
	})

	t.Run("PROMPTに明示的に指定されたドットファイルは例外としてスキップされないこと", func(t *testing.T) {
		// プロジェクトルート
		gitRoot := CreateTestGitRoot(t)

		// ルートに隠しファイルを配置
		envPath := WriteTestFile(t, gitRoot, ".env", "A=B")

		// walkDir中のカレントディレクトリ
		d := GetDirEntry(t, envPath)

		// promptPatterns に指定がある場合
		promptPatterns := []string{".env"}

		skip, err := shouldSkipEntry(envPath, d, gitRoot, []string{}, promptPatterns)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if skip {
			t.Errorf("PROMPTに指定されたドットファイルは許可されるべきです")
		}
	})

	t.Run("ドットディレクトリは配下ごとスキップ（SkipDir）されること", func(t *testing.T) {
		// プロジェクトルート
		gitRoot := CreateTestGitRoot(t)

		// ルートに隠しディレクトリとその中のファイルを配置
		dotDir := MakeTestDir(t, gitRoot, ".project")
		WriteTestFile(t, dotDir, "a.txt", "contents") // 直接触らないので返り値は捨てる

		// walkDir中のディレクトリとして、隠しファイルの場所→FileInfo→DirEntryと変換
		dInfo, _ := os.Stat(dotDir.String())
		d := fs.FileInfoToDirEntry(dInfo)

		_, dirErr := shouldSkipEntry(dotDir, d, gitRoot, []string{}, []string{})
		if dirErr != filepath.SkipDir {
			t.Errorf("期待値: filepath.SkipDir, 実際の戻り値: %v", dirErr)
		}
	})
}
func TestShouldSkipEntry_ProjcatIgnoreDir(t *testing.T) {
	t.Run(".projcatignoreの末尾スラッシュワイルドカード記述で、深い階層のディレクトリも漏れなくスキップされること", func(t *testing.T) {
		gitRoot := CreateTestGitRoot(t)

		// 無視対象として指定するディレクトリの、さらに深いサブディレクトリを作成
		// 例として「internal/*」が無視対象のとき、「internal/app/utils」という深い階層を用意
		deepDir := MakeTestDir(t, gitRoot, "internal/app/utils")
		d := GetDirEntry(t, deepDir)

		// ignorePatterns に「internal/*」をシミュレート
		ignorePatterns := []string{"internal/*"}

		// shouldSkipEntry を呼び出す
		// 現在の実装（完全一致のみ）だと、"internal/app/utils" は "internal" にマッチしないため skip=false になってしまう
		_, err := shouldSkipEntry(deepDir, d, gitRoot, ignorePatterns, []string{})

		if err != filepath.SkipDir {
			t.Errorf("無視対象 'internal/*' の配下にあるディレクトリ 'internal/app/utils' はスキップされるべきです")
		}
	})
}

func TestShouldIgnoreWithPatterns_DeepDir(t *testing.T) {
	t.Run("shouldIgnoreWithPatterns関数が/*パターンの深い階層のディレクトリに対して正しくtrueとSkipDirを返すこと", func(t *testing.T) {
		ignorePatterns := []string{"internal/*"}

		// 深い階層のディレクトリパス
		gitRoot := CreateTestGitRoot(t)
		targetDir := MakeTestDir(t, gitRoot, "internal/app/utils")

		// shouldIgnoreWithPatterns(path, isDir, patterns)
		ignored := shouldIgnoreWithPatterns(targetDir, ignorePatterns)

		if !ignored {
			t.Errorf("期待値: ignored = true, 実際の値: false")
		}
	})
}
