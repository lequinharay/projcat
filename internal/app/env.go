package app

import (
	"fmt"
	"os/exec"
)

// ツールの実行に必要な外部コマンドが揃っているかの検証
func ValidateEnvironment(opts *RunOptions) error {
	// 常に必須の Git チェック
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("必須コマンド 'git' が見つかりません。Gitがインストールされ、PATHが通っているか確認してください")
	}

	// オプション指定時のみ必須の pbcopy チェック
	if opts != nil && opts.Clipboard {
		if _, err := exec.LookPath("pbcopy"); err != nil {
			return fmt.Errorf("クリップボード機能（pbcopy）が利用できません。macOS環境でのみサポートされています")
		}
	}

	return nil
}
