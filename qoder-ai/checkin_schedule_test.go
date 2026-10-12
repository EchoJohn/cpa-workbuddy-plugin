package main

import (
	"testing"
	"time"
)

// TestCheckinHoursAreTenAndTwentyTwo 锁定 2026-10-12 的改造契约：签到从每 4 小时
// 收敛为每天 10:00 与 22:00 两次，避免在上游 billing 接口上产生无谓调用。
func TestCheckinHoursAreTenAndTwentyTwo(t *testing.T) {
	if len(checkinHours) != 2 || checkinHours[0] != 10 || checkinHours[1] != 22 {
		t.Fatalf("checkinHours = %v, want [10 22]", checkinHours)
	}
}

// TestShouldRunCheckinNow 验证签到只在 checkinHours 时段触发：共享定时器会被
// keepalive 时刻唤醒，若不门控则签到会在那些时刻重复执行。
func TestShouldRunCheckinNow(t *testing.T) {
	loc := time.Now().Location()
	cases := []struct {
		name string
		hour int
		min  int
		want bool
	}{
		{"09:59 未到签到时段", 9, 59, false},
		{"10:00 签到起点", 10, 0, true},
		{"10:59 一小时内仍触发", 10, 59, true},
		{"11:00 时段结束边界", 11, 0, false},
		{"21:59 未到晚间时段", 21, 59, false},
		{"22:00 晚间签到起点", 22, 0, true},
		{"22:30 晚间时段内", 22, 30, true},
		{"23:00 晚间时段结束边界", 23, 0, false},
		{"04:00 keepalive 时刻不得触发签到", 4, 0, false},
		{"16:00 keepalive 时刻不得触发签到", 16, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 10, 12, tc.hour, tc.min, 0, 0, loc)
			if got := shouldRunCheckinNow(now); got != tc.want {
				t.Fatalf("shouldRunCheckinNow(%02d:%02d) = %v, want %v", tc.hour, tc.min, got, tc.want)
			}
		})
	}
}

// TestNextCheckinTimeSkipsKeepaliveOnlySlots 验证共享定时器仍会为 keepalive
// 时刻唤醒，而签到由门控负责过滤——两者职责分离。
func TestNextCheckinTimeSkipsKeepaliveOnlySlots(t *testing.T) {
	loc := time.Now().Location()
	// 09:30 之后的下一个唤醒点应为 10:00（签到），而不是 12:00（keepalive）。
	now := time.Date(2026, 10, 12, 9, 30, 0, 0, loc)
	next := nextCheckinTime(now)
	want := time.Date(2026, 10, 12, 10, 0, 0, 0, loc)
	if !next.Equal(want) {
		t.Fatalf("nextCheckinTime(09:30) = %v, want %v", next, want)
	}
}
