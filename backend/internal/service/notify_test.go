package service

import "testing"

// TestGenSign 签名算法回归：与 quant-engine/app/notify.py gen_sign 同算法
// 固定输入基准值（与 Python 侧交叉验证一致）：
//
//	secret=s3cr3t ts=1620000000 →
//	CUn/OkC1u4aXSq7M6nIjKZYEb0gEQFJjRVQy/LgqQ14=
func TestGenSign(t *testing.T) {
	got := genSign("s3cr3t", 1620000000)
	want := "CUn/OkC1u4aXSq7M6nIjKZYEb0gEQFJjRVQy/LgqQ14="
	if got != want {
		t.Fatalf("签名不符: got %q want %q", got, want)
	}
}
