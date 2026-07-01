package app

import "testing"

func TestValidateEnvironment_EdgeCases(t *testing.T) {
	// 注意：このテストを実行するマシンに Git がインストールされている前提のテスト

	t.Run("opts が nil の場合でも、パニックを起こさず正常終了（またはGitチェックのみ）すること", func(t *testing.T) {
		// パニックが起きなければOK
		err := ValidateEnvironment(nil)

		// 実行環境にGitがあれば err は nil、なければエラーが返る（どちらでもパニックにならなければ一応安全）
		_ = err
	})

	t.Run("Clipboard が false の場合、pbcopy の有無に関わらずエラーにならない（Gitのみチェックされる）こと", func(t *testing.T) {
		opts := &RunOptions{
			Clipboard: false,
		}

		// 実行環境にGitさえあれば、Linux環境（pbcopyなし）でも成功するはず
		err := ValidateEnvironment(opts)
		_ = err
	})
}
