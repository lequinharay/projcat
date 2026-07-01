package app

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// ParseArgs はコマンドライン引数のスライスを受け取り、対象ディレクトリのパスを決定する
func ParseArgs(args []string) string {
	// 引数が何も指定されていない場合は、カレントディレクトリ（.）を返す
	if len(args) == 0 {
		return "."
	}

	// 第1引数が指定されている場合は、その値をそのまま返す
	return args[0]
}

// ValidateTargetDir は指定されたパスが有効なGitリポジトリの内部か判定する
func ValidateTargetDir(target string) error {
	// 指定されたディレクトリが存在するか、まず事前チェック
	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("指定されたディレクトリが存在しません: %w", err)
	}

	// 指定ディレクトリで git rev-parse を実行し、そのパスがGit管理下にあるかをGitに直接判定させる
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = target

	// 実行結果のエラー（終了コード非0）が出るかどうかだけを検証
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("指定された場所はGitリポジトリの内部ではありません（またはgitコマンドが利用できません）: %w", err)
	}

	return nil
}

// ScanAndAggregate は、指定されたディレクトリ配下を走査し、
// Git無視対象外のテキストファイルを1つのMarkdownに結合した文字列を返す
// この関数はmain.goから直接呼ばれるので、ValueObjectで引数を受けられない
func ScanAndAggregate(targetDir CanonicalPath, opts *RunOptions) (string, error) {
	if opts == nil {
		opts = &RunOptions{}
	}

	/* 事前準備 */
	// Gitルートディレクトリを基準にした相対パスを取得
	gitRootDir, err := getGitRoot(targetDir)
	if err != nil {
		return "", err
	}

	// .projcatignore をロードする
	ignorePatterns, err := loadProjcatIgnore(gitRootDir)
	if err != nil {
		return "", err
	}
	// .projpromptfiles をロードする
	promptPatterns, err := loadProjcatPromptFiles(gitRootDir)
	if err != nil {
		return "", fmt.Errorf(".projcatpromptfiles の読み込みに失敗: %w", err)
	}

	// プロジェクト名を推定する
	projectName := estimateProjectName(targetDir)

	// 最終的に出力する為のバッファを用意し、タイトル行を追加
	var result bytes.Buffer
	result.WriteString(fmt.Sprintf("# %s\n\n", projectName))

	/* 特別なファイルの処理 */
	specialSections := []struct {
		title string
		file  string
	}{
		{"README", "README.md"},
		{"SPEC", "SPEC.md"},
		{"ROADMAP", "ROADMAP.md"},
	}

	for _, s := range specialSections {
		// gitRootDir からの相対的な位置を計算するために一時的に結合するが、
		// 存在チェックと読み込みの前に必ず ToCanonicalPath で評価する
		specialPathStr := filepath.Join(gitRootDir.String(), s.file)
		if specialPath, err := ToCanonicalPath(specialPathStr); err == nil && specialPath.Exists() {
			content, err := os.ReadFile(specialPath.String())
			if err == nil {
				fence := determineFence(string(content))
				result.WriteString(fmt.Sprintf("## %s\n%s%s\n%s\n%s\n\n", s.title, fence, s.file, string(content), fence))
			}
		}
	}

	/* ディレクトリツリー文字列の生成 */
	treeString, err := buildDirectoryTree(gitRootDir, ignorePatterns, promptPatterns)
	if err != nil {
		return "", err
	}
	treeFence := determineFence(treeString)
	result.WriteString(fmt.Sprintf("## DIR\n%s\n%s%s\n\n", treeFence, treeString, treeFence))

	// 各動的セクションのバッファ
	var promptContent bytes.Buffer
	var filesContent bytes.Buffer

	/**
	 * ディレクトリを再帰的に走査する
	 */
	err = filepath.WalkDir(targetDir.String(), func(pathString string, d os.DirEntry, err error) error {
		// エラーが発生した場合は、そのまま返す
		if err != nil {
			return err
		}

		path, _ := ToCanonicalPath(pathString) // 型安全に CanonicalPath へ変換
		skip, walkErr := shouldSkipEntry(path, d, gitRootDir, ignorePatterns, promptPatterns)
		if walkErr != nil {
			return walkErr
		}
		if skip {
			return nil
		}

		// ディレクトリ自体はファイルコンテンツにしない
		if d.IsDir() {
			return nil
		}

		gitRel := path.RelToGitRoot(gitRootDir)
		gitRelStr := gitRel.String()

		// 設定ファイル自体（.projcatignore 等）は FILES や PROMPT に含めない
		if gitRelStr == ".projcatignore" || gitRelStr == ".projcatpromptfiles" {
			return nil
		}

		if isSpecialFile(gitRel) {
			return nil
		}

		relToTarget, err := filepath.Rel(targetDir.String(), path.String())
		if err != nil || strings.HasPrefix(relToTarget, "..") {
			return nil
		}

		// PROMPT セクションへの追加判定
		if isMatchedPromptFile(gitRelStr, promptPatterns) {
			block, err := buildFileContentBlock(path, gitRel)
			if err != nil {
				return err
			}
			if promptContent.Len() > 0 {
				promptContent.WriteString("\n")
			}
			promptContent.WriteString(block)
		}

		// FILES セクションへの追加
		if !isMatchedFilter(gitRelStr, opts.Filter) {
			return nil // フィルターにマッチしない場合はFILESに含めずスキップ
		}

		block, err := buildFileContentBlock(path, gitRel)
		if err != nil {
			return err
		}
		if filesContent.Len() > 0 {
			filesContent.WriteString("\n")
		}
		filesContent.WriteString(block)

		return nil
	})

	// 走査中にエラーが発生した場合は、そのまま返す
	if err != nil {
		return "", err
	}

	// ## PROMPT セクションの書き出し（中身がある場合、または空でもセクション自体は出すか）
	// テスト要件に合わせて、常にセクションヘッダーを出力します
	result.WriteString("## PROMPT\n")
	if promptContent.Len() > 0 {
		result.WriteString(promptContent.String())
		result.WriteString("\n\n")
	} else {
		result.WriteString("\n")
	}

	// ## FILES セクションの書き出し
	result.WriteString("## FILES\n")
	result.WriteString(filesContent.String())

	if opts.Clipboard {
		if err := clipboardWriter(result.String()); err != nil {
			return "", fmt.Errorf("クリップボードへのコピーに失敗しました: %w", err)
		}

		return "Copied!\n", nil
	}

	return result.String(), nil
}

// getGitRoot は指定されたターゲットディレクトリが属するGitリポジトリのルートパス（絶対パス）を取得する
func getGitRoot(target CanonicalPath) (CanonicalPath, error) {
	if !target.Exists() {
		return CanonicalPath{}, fmt.Errorf("指定のパスが存在しません: %s", target.String())
	}
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = target.String()
	out, err := cmd.Output()
	if err != nil {
		return CanonicalPath{}, fmt.Errorf("Gitルートディレクトリの取得に失敗しました: %w", err)
	}

	// 取得したルートパスも即座に型安全な CanonicalPath に変換する
	return ToCanonicalPath(strings.TrimSpace(string(out)))
}

// loadProjcatIgnore は、ターゲットディレクトリ内の .projcatignore を読み込む
func loadProjcatIgnore(gitRoot CanonicalPath) ([]string, error) {
	path := filepath.Join(gitRoot.String(), ".projcatignore")
	return loadConfigPatterns(path)
}

// loadProjcatPromptFiles は、ターゲットディレクトリ内の .projcatpromptfiles を読み込む（存在しなくてもエラーにしない）
func loadProjcatPromptFiles(gitRoot CanonicalPath) ([]string, error) {
	path := filepath.Join(gitRoot.String(), ".projcatpromptfiles")
	return loadConfigPatterns(path)
}

// loadConfigPatterns は指定された設定ファイルから有効なパターン行を読み込みます
func loadConfigPatterns(pathStr string) ([]string, error) {
	content, err := os.ReadFile(pathStr)
	if err != nil {
		// ファイルが存在しない場合は正常系として空リストを返す
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var lines []string
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	return lines, nil
}

// estimateProjectName はパスからプロジェクト名を推定する
func estimateProjectName(target CanonicalPath) string {
	gitRoot, _ := getGitRoot(target)

	return filepath.Base(gitRoot.String())
}

// determineFence はコンテンツ内の最大連続バックティック数を走査し、安全な最外枠のフェンス文字列を決定する
func determineFence(content string) string {
	maxCount := 0
	currentCount := 0

	// 文字列を1文字ずつ走査（Goの rune ループ）
	for _, char := range content {
		if char == '`' {
			currentCount++
			if currentCount > maxCount {
				maxCount = currentCount
			}
		} else {
			currentCount = 0
		}
	}

	// 最低でも4つのバックティックを担保する（仕様：最外枠は常に4つ以上）
	fenceCount := maxCount + 1
	if fenceCount < 4 {
		fenceCount = 4
	}

	// 指定した文字を指定回数リピートした文字列を生成して返す
	return strings.Repeat("`", fenceCount)
}

// buildDirectoryTree は指定されたディレクトリ配下の有効なファイルからツリー構造テキストを生成する
func buildDirectoryTree(gitRootDir CanonicalPath, ignorePatterns []string, promptPatterns []string) (string, error) {
	var paths []string

	err := filepath.WalkDir(gitRootDir.String(), func(pathStr string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		path, _ := ToCanonicalPath(pathStr) // 型安全に CanonicalPath へ変換
		skip, walkErr := shouldSkipEntry(path, d, gitRootDir, ignorePatterns, promptPatterns)
		if walkErr != nil {
			return walkErr
		}
		if skip {
			return nil // ファイル単体スキップなど用
		}

		// ツリー構造としてのレンダリング用相対パス（targetDir 起点）
		targetRel, err := filepath.Rel(gitRootDir.String(), path.String())
		if err != nil {
			return nil
		}

		// ルートディレクトリ自体はツリー構造に含めない
		if targetRel == "." {
			return nil
		}

		paths = append(paths, targetRel)
		return nil
	})

	if err != nil {
		return "", err
	}

	// sort.Strings(paths)

	type Node struct {
		Name     string
		Children map[string]*Node
		IsDir    bool
	}

	root := &Node{Children: make(map[string]*Node), IsDir: true}

	for _, p := range paths {
		parts := strings.Split(filepath.ToSlash(p), "/")
		current := root
		for i, part := range parts {
			isLast := i == len(parts)-1
			if _, exists := current.Children[part]; !exists {
				current.Children[part] = &Node{
					Name:     part,
					Children: make(map[string]*Node),
					IsDir:    !isLast,
				}
			}

			// ループの途中（末尾ではない）＝確実にディレクトリなので、
			// 過去にファイルとして誤登録されていた場合でもここで安全にディレクトリへ補正する
			if !isLast {
				current.Children[part].IsDir = true
			}
			current = current.Children[part]
		}
	}

	var renderTree func(node *Node, prefixes []string) string
	renderTree = func(node *Node, prefixes []string) string {
		var sb strings.Builder

		var keys []string
		for k := range node.Children {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for i, k := range keys {
			child := node.Children[k]
			isLast := i == len(keys)-1

			for _, pref := range prefixes {
				sb.WriteString(pref)
			}
			if isLast {
				sb.WriteString("└── ")
			} else {
				sb.WriteString("├── ")
			}
			sb.WriteString(child.Name)
			sb.WriteString("\n")

			if len(child.Children) > 0 {
				nextPrefixes := append([]string{}, prefixes...)
				if isLast {
					nextPrefixes = append(nextPrefixes, "    ")
				} else {
					nextPrefixes = append(nextPrefixes, "│   ")
				}
				sb.WriteString(renderTree(child, nextPrefixes))
			}
		}
		return sb.String()
	}

	return strings.TrimSuffix(renderTree(root, []string{}), "\n"), nil
}

// shouldSkipEntry は、現在のパスが走査からスキップまたは除外されるべきかを判定する。
// 戻り値の error に filepath.SkipDir が返る場合は、そのディレクトリ配下を丸ごとスキップする。
func shouldSkipEntry(path CanonicalPath, d os.DirEntry, gitRootDir CanonicalPath, ignorePatterns []string, promptPatterns []string) (bool, error) {
	// ドットディレクトリ・ドットファイルの原則スキップ処理 */
	name := d.Name()
	if strings.HasPrefix(name, ".") {
		if d.IsDir() {
			// .git を含む、すべてのドットディレクトリ配下を丸ごとスキップ
			return true, filepath.SkipDir
		}

		// 例外：PROMPTセクションに明示的に指定されている場合はスキップしない
		targetRel := path.RelToGitRoot(gitRootDir)
		if isMatchedPromptFile(targetRel.String(), promptPatterns) {
			return false, nil
		}
		// 例外に漏れたドットファイルは原則通り省略
		return true, nil
	}

	// 型安全にGit相対パスへ変換
	targetRel := path.RelToGitRoot(gitRootDir)

	/* ディレクトリ自体が無視対象に指定されているかチェック */
	if d.IsDir() {
		// .gitignore 等で Git 側から無視されているディレクトリか
		if isIgnoredByGit(targetRel, gitRootDir, true) {
			return true, filepath.SkipDir // 中身ごとスキップ
		}

		// ディレクトリの段階で早期スキップ（filepath.SkipDir）させるための判定
		for _, pattern := range ignorePatterns {
			// .projcatignore に `dirname/*` と書かれているときのみディレクトリ扱い
			if strings.HasSuffix(pattern, "/*") {
				dirPattern := strings.TrimSuffix(pattern, "/*")
				// 自身がそのディレクトリそのもの、またはその配下階層であるか
				if targetRel.String() == dirPattern || strings.HasPrefix(targetRel.String(), dirPattern+"/") {
					return true, filepath.SkipDir
				}
			}
		}
		return false, nil
	}

	/* ファイルに対する判定 */
	// .projcatignore および .gitignore 自体は出力に含めないためスキップ
	if targetRel.String() == ".projcatignore" || targetRel.String() == ".gitignore" {
		return true, nil
	}

	// Gitの無視対象にヒットする場合はスキップする
	if isIgnoredByGit(targetRel, gitRootDir, false) {
		return true, nil
	}

	// プロジェクト固有の無視設定
	// プロジェクト固有の無視設定
	if shouldIgnoreWithPatterns(path, ignorePatterns) {
		if d.IsDir() {
			return true, filepath.SkipDir
		}
		return true, nil
	}

	return false, nil
}

// isIgnoredByGit は、本物の git check-ignore コマンドを呼び出して
// 指定されたパスがGitの無視対象かどうかを判定する
func isIgnoredByGit(targetPath gitRelPath, gitRoot CanonicalPath, isDir bool) bool {
	if !targetPath.Exists() {
		// 存在しないなら無視して良い
		return true
	}
	pathStr := targetPath.String()
	// 対象がディレクトリの場合、またはパスがスラッシュで終わっていない場合は末尾にスラッシュを付加する
	// これにより、ディスク上に実体がない仮想パスであってもGit側でディレクトリパターン（例: .history/）にマッチする
	if isDir && !strings.HasSuffix(pathStr, "/") {
		pathStr += "/"
	}

	// git check-ignore は、Gitルート直下で、ルートからの相対パスを指定する
	cmd := exec.Command("git", "check-ignore", "-q", targetPath.String())
	cmd.Dir = gitRoot.String()

	_, err := cmd.CombinedOutput()

	/**
	 * Goの方言: コマンドの終了コードが0以外の場合、Run() は error を返す
	 * git check-ignore は「無視対象ではない」ときに終了コード 1 (error) になるため、
	 * エラーがある = 無視対象ではない (false)
	 */
	return err == nil
}

// buildFileContentBlock はファイルを読み込み、動的フェンスで囲まれたMarkdownブロックを作成する
func buildFileContentBlock(path CanonicalPath, displayFilename gitRelPath) (string, error) {
	content, err := os.ReadFile(path.String())
	if err != nil {
		return "", err
	}
	fence := determineFence(string(content))

	return fmt.Sprintf("%s%s\n%s\n%s", fence, displayFilename.String(), string(content), fence), nil
}

// shouldIgnoreWithPatterns は、指定されたパスが無視パターンのいずれかにマッチするか判定する
func shouldIgnoreWithPatterns(path CanonicalPath, patterns []string) bool {
	base := path.Base()

	for _, pattern := range patterns {
		// 空文字やコメント行（#で始まる行）は無視する
		if pattern == "" || strings.HasPrefix(pattern, "#") {
			continue
		}

		// 末尾の "/*" や "/" をトリミングしてディレクトリ前方一致を判定
		if strings.HasSuffix(pattern, "/*") {
			dirPattern := strings.TrimSuffix(pattern, "/*")
			if path.String() == dirPattern || strings.Contains(path.Dir(), dirPattern+"/") {
				return true
			}
			continue
		}

		// 拡張子指定（例: *.png）への後方一致
		if strings.HasPrefix(pattern, "*.") {
			ext := strings.TrimPrefix(pattern, "*")
			if strings.HasSuffix(path.String(), ext) {
				return true
			}
			continue
		}

		// ワイルドカードがない単純記述ルール
		if path.String() == pattern || base == pattern {
			return true
		}

		// filepath.Match による標準的なワイルドカード判定
		if matched, err := filepath.Match(pattern, path.String()); err == nil && matched {
			return true
		}
	}

	return false
}

// isSpecialFile は、「特別な説明」や「PROMPT」などの固定セクション対象ファイルか判定する
func isSpecialFile(relPath gitRelPath) bool {
	switch relPath.String() {
	case "README.md", "SPEC.md", "ROADMAP.md":
		return true
	}
	if strings.HasSuffix(relPath.String(), "rules") && strings.HasPrefix(relPath.String(), ".") {
		return true
	}
	return false
}

// isMatchedPromptFile はファイルが .projcatpromptfiles のいずれかのルールに合致するか判定します
func isMatchedPromptFile(gitRelStr string, promptPatterns []string) bool {
	base := filepath.Base(gitRelStr)

	for _, pattern := range promptPatterns {
		// ディレクトリ指定（末尾が /*）の場合
		if strings.HasSuffix(pattern, "/*") {
			dirPrefix := strings.TrimSuffix(pattern, "/*")
			// 対象ファイルがそのディレクトリ配下（階層不問で前方一致）にあるか
			// 例: pattern "prompts/*" に対して gitRelStr "prompts/sub/rules.txt" はマッチ
			if gitRelStr == dirPrefix || strings.HasPrefix(gitRelStr, dirPrefix+"/") {
				return true
			}
		} else {
			// 単一指定の場合：ルートからの相対パス完全一致、またはファイル名（Base）完全一致
			if gitRelStr == pattern || base == pattern {
				return true
			}
		}
	}
	return false
}

// isMatchedFilter は、指定された相対パスがフィルター条件に合致するか判定する
// フィルターが空の場合は常に true を返す
func isMatchedFilter(gitRelStr string, filter string) bool {
	if filter == "" {
		return true
	}

	// カンマ区切りで分割し、いずれかのキーワードが相対パスに含まれているか判定 (OR条件)
	keywords := strings.Split(filter, ",")
	for _, kw := range keywords {
		kw = strings.TrimSpace(kw)
		if kw == "" {
			continue
		}
		if strings.Contains(gitRelStr, kw) {
			return true // 1つでもヒットすればOK
		}
	}

	return false
}

// クリップボード書き込み用。モック化するために無名関数を使う
var clipboardWriter = func(text string) error {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}
