package market

import (
	"testing"
	"time"
)

func TestIsUSMarketOpen(t *testing.T) {
	ny, _ := time.LoadLocation("America/New_York")

	cases := []struct {
		name string
		t    time.Time
		want bool
	}{
		{
			name: "정규장 중간 (Mon 10:00 ET)",
			t:    time.Date(2026, 3, 16, 10, 0, 0, 0, ny), // Monday
			want: true,
		},
		{
			name: "장 시작 정각 (09:30 ET)",
			t:    time.Date(2026, 3, 16, 9, 30, 0, 0, ny),
			want: true,
		},
		{
			name: "장 마감 정각 (16:00 ET) — 마감 후",
			t:    time.Date(2026, 3, 16, 16, 0, 0, 0, ny),
			want: false,
		},
		{
			name: "프리마켓 (09:29 ET)",
			t:    time.Date(2026, 3, 16, 9, 29, 0, 0, ny),
			want: false,
		},
		{
			name: "장 후 (17:00 ET)",
			t:    time.Date(2026, 3, 16, 17, 0, 0, 0, ny),
			want: false,
		},
		{
			name: "토요일",
			t:    time.Date(2026, 3, 14, 11, 0, 0, 0, ny),
			want: false,
		},
		{
			name: "일요일",
			t:    time.Date(2026, 3, 15, 11, 0, 0, 0, ny),
			want: false,
		},
		{
			name: "뉴이어 공휴일 (2026-01-01 10:00 ET)",
			t:    time.Date(2026, 1, 1, 10, 0, 0, 0, ny),
			want: false,
		},
		{
			name: "MLK Day (2026-01-19 10:00 ET)",
			t:    time.Date(2026, 1, 19, 10, 0, 0, 0, ny),
			want: false,
		},
		{
			name: "서머타임 중 정규장 (2026-06-01 Mon 10:00 ET)",
			t:    time.Date(2026, 6, 1, 10, 0, 0, 0, ny),
			want: true,
		},
		{
			name: "크리스마스 (2026-12-25)",
			t:    time.Date(2026, 12, 25, 11, 0, 0, 0, ny),
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsUSMarketOpen(tc.t)
			if got != tc.want {
				t.Errorf("IsUSMarketOpen(%v) = %v, want %v", tc.t, got, tc.want)
			}
		})
	}
}
