package app

// RunOptions は走査時の動的な挙動を制御する設定オブジェクト
type RunOptions struct {
	Filter    string // カンマ区切りの部分一致フィルター
	Clipboard bool   // trueならpbcopyに結果を入れる
}
