package app

import (
	"strings"
	"testing"
)

func TestScanAndAggregate_Options(t *testing.T) {
	t.Run("--filter オプションによって FILES セクションのみがカンマ区切りのOR条件で部分一致フィルタリングされること", func(t *testing.T) {
		gitRoot := CreateTestGitRoot(t)

		/* テスト用のファイル群を配置 */
		// マッチ対象
		WriteTestFile(t, gitRoot, "user_service.go", "User Service")
		WriteTestFile(t, gitRoot, "auth_handler.go", "Auth Handler")

		// マッチ対象外
		WriteTestFile(t, gitRoot, "main.go", "Main Content")

		// PROMPT用の設定とファイル（フィルターの影響を「受けない」ことを検証するため）
		WriteTestFile(t, gitRoot, ".projcatpromptfiles", "main.go\n")

		// フィルターオプションを指定して実行
		opts := &RunOptions{
			Filter: "user,handler",
		}

		result, err := ScanAndAggregate(gitRoot, opts)
		if err != nil {
			t.Fatalf("予期せぬエラー: %v", err)
		}

		/* 検証 */
		promptIdx := strings.Index(result, "## PROMPT")
		filesIdx := strings.Index(result, "## FILES")
		if promptIdx == -1 || filesIdx == -1 {
			t.Fatalf("必要なセクションが含まれていません")
		}

		// ## PROMPT セクションはフィルターの影響を受けず、main.go が含まれていること
		promptSection := result[promptIdx:filesIdx]
		if !strings.Contains(promptSection, "main.go") {
			t.Errorf("PROMPT セクションがフィルターによって誤って除外されています")
		}

		// ## FILES セクション内を検証
		filesSection := result[filesIdx:]
		if !strings.Contains(filesSection, "user_service.go") {
			t.Errorf("フィルター条件 'user' にマッチするファイルが含まれていません")
		}
		if !strings.Contains(filesSection, "auth_handler.go") {
			t.Errorf("フィルター条件 'handler' にマッチするファイルが含まれていません")
		}
		if strings.Contains(filesSection, "main.go") {
			t.Errorf("フィルター条件にマッチしない 'main.go' が FILES セクションに含まれてしまっています")
		}
	})
}

func TestRunOptions_Clipboard(t *testing.T) {
	cPath := CreateTestGitRoot(t)

	t.Run("Clipboardオプションが有効な場合、正常に処理が完了すること", func(t *testing.T) {
		opts := &RunOptions{
			Clipboard: true,
		}

		// まだ実装していないため、内部で pbcopy を呼ぶロジックが未実装、
		// あるいはクリップボードへの流し込みフラグが立っている場合の挙動を検証
		result, err := ScanAndAggregate(cPath, opts)
		if err != nil {
			t.Fatalf("Clipboard指定時の実行に失敗: %v", err)
		}

		// クリップボードに送る場合、標準出力用の result は空文字、
		// もしくは元のテキストのどちらにすべきか？（通常は空文字にして、完了メッセージだけ出すのが親切です）
		expected := "Copied!\n"
		if result != expected {
			t.Errorf("期待値: %q, 実際の戻り値: %q", expected, result)
		}
	})
}
