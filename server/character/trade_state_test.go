package character

// STAGE-294：交易状态机的单元测试（docs/trade.md 2.3、4.3）。

import (
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/protocol"
)

func newTestTrade(t *testing.T) (*trade, time.Time) {
	t.Helper()
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	a := &tradeSide{CharID: 1, Name: "甲"}
	b := &tradeSide{CharID: 2, Name: "乙"}
	return newTrade("t-1", a, b, now), now
}

func wantCode(t *testing.T, err *protocol.Error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want %s, got nil", code)
	}
	if err.Code != code {
		t.Fatalf("want %s, got %s (%s)", code, err.Code, err.Message)
	}
}

func wantOK(t *testing.T, err *protocol.Error) {
	t.Helper()
	if err != nil {
		t.Fatalf("want ok, got %s (%s)", err.Code, err.Message)
	}
}

func TestTradeNewStartsEditing(t *testing.T) {
	tr, _ := newTestTrade(t)
	if tr.Rev != 1 {
		t.Fatalf("rev = %d, want 1", tr.Rev)
	}
	for _, s := range tr.Sides {
		if s.Offer.Pet != -1 || len(s.Offer.Items) != 0 || s.Offer.Gold != 0 {
			t.Fatalf("new offer not empty: %+v", s.Offer)
		}
	}
}

func TestOfferShape(t *testing.T) {
	cases := []struct {
		name  string
		offer tradeOffer
		code  string
	}{
		{"empty ok", tradeOffer{Pet: -1}, ""},
		{"two items ok", tradeOffer{Items: []int{5, 19}, Pet: -1}, ""},
		{"three items", tradeOffer{Items: []int{5, 6, 7}, Pet: -1}, protocol.CodeBadRequest},
		{"equip slot", tradeOffer{Items: []int{4}, Pet: -1}, protocol.CodeBadRequest},
		{"slot 20", tradeOffer{Items: []int{20}, Pet: -1}, protocol.CodeBadRequest},
		{"duplicate slot", tradeOffer{Items: []int{8, 8}, Pet: -1}, protocol.CodeBadRequest},
		{"negative gold", tradeOffer{Gold: -1, Pet: -1}, protocol.CodeBadRequest},
		{"pet 20", tradeOffer{Pet: MaxPetSlots}, protocol.CodeBadRequest},
		{"pet 19 ok", tradeOffer{Pet: MaxPetSlots - 1}, ""},
	}
	for _, c := range cases {
		err := checkOfferShape(c.offer)
		if c.code == "" {
			wantOK(t, err)
		} else {
			wantCode(t, err, c.code)
		}
	}
}

func TestOfferChangeBumpsRevAndClearsMarks(t *testing.T) {
	tr, now := newTestTrade(t)
	wantOK(t, tr.lock(1, 1, now, nil))
	wantOK(t, tr.lock(2, 1, now, nil))
	// 双方都锁定后，任何一方改报价：rev +1，锁定清零
	wantOK(t, tr.setOffer(2, tradeOffer{Gold: 100, Pet: -1}, now))
	if tr.Rev != 2 {
		t.Fatalf("rev = %d, want 2", tr.Rev)
	}
	if tr.side(1).Locked || tr.side(2).Locked {
		t.Fatal("locks must be cleared after an offer change")
	}
}

func TestLockNeedsCurrentRev(t *testing.T) {
	tr, now := newTestTrade(t)
	wantOK(t, tr.setOffer(1, tradeOffer{Gold: 5, Pet: -1}, now)) // rev 2
	wantCode(t, tr.lock(2, 1, now, nil), protocol.CodeTradeStale)
	wantOK(t, tr.lock(2, 2, now, nil))
}

func TestLockPreflightRejects(t *testing.T) {
	tr, now := newTestTrade(t)
	reject := func() *protocol.Error { return protocol.Errorf(protocol.CodeBagFull, "对方背包满") }
	wantCode(t, tr.lock(1, 1, now, reject), protocol.CodeBagFull)
	if tr.side(1).Locked {
		t.Fatal("a rejected preflight must not lock")
	}
}

func TestLockIsIdempotentAndDoesNotBump(t *testing.T) {
	tr, now := newTestTrade(t)
	wantOK(t, tr.lock(1, 1, now, nil))
	wantOK(t, tr.lock(1, 1, now, nil))
	if tr.Rev != 1 {
		t.Fatalf("lock must not bump rev, got %d", tr.Rev)
	}
}

func TestConfirmNeedsBothLocked(t *testing.T) {
	tr, now := newTestTrade(t)
	wantOK(t, tr.lock(1, 1, now, nil))
	done, perr := tr.confirm(1, 1, now)
	wantCode(t, perr, protocol.CodeWrongState)
	if done {
		t.Fatal("must not be done")
	}
}

func TestConfirmTwoPartiesCompletes(t *testing.T) {
	tr, now := newTestTrade(t)
	wantOK(t, tr.lock(1, 1, now, nil))
	wantOK(t, tr.lock(2, 1, now, nil))
	done, perr := tr.confirm(1, 1, now)
	wantOK(t, perr)
	if done {
		t.Fatal("first confirm must not complete")
	}
	done, perr = tr.confirm(2, 1, now)
	wantOK(t, perr)
	if !done {
		t.Fatal("second confirm must complete")
	}
}

func TestConfirmStaleRevAfterPeerChange(t *testing.T) {
	tr, now := newTestTrade(t)
	wantOK(t, tr.lock(1, 1, now, nil))
	wantOK(t, tr.lock(2, 1, now, nil))
	// 乙在甲确定之前改了报价：甲带旧 rev 确定会失败，双方锁定已清零
	wantOK(t, tr.setOffer(2, tradeOffer{Gold: 1, Pet: -1}, now))
	_, perr := tr.confirm(1, 1, now)
	wantCode(t, perr, protocol.CodeTradeStale)
}

func TestSecondConfirmAfterFirstIsNotDuplicateCompletion(t *testing.T) {
	tr, now := newTestTrade(t)
	wantOK(t, tr.lock(1, 1, now, nil))
	wantOK(t, tr.lock(2, 1, now, nil))
	_, _ = tr.confirm(1, 1, now)
	// 同一边重复确定：不会再次完成
	done, perr := tr.confirm(1, 1, now)
	wantOK(t, perr)
	if done {
		t.Fatal("repeating one side's confirm must not complete the trade")
	}
}

func TestUnlockClearsBothAndBumps(t *testing.T) {
	tr, now := newTestTrade(t)
	wantOK(t, tr.lock(1, 1, now, nil))
	wantOK(t, tr.lock(2, 1, now, nil))
	_, _ = tr.confirm(1, 1, now)
	wantOK(t, tr.unlock(2, now))
	if tr.Rev != 2 {
		t.Fatalf("rev = %d, want 2", tr.Rev)
	}
	for _, s := range tr.Sides {
		if s.Locked || s.Confirmed {
			t.Fatalf("side %d still marked after unlock", s.CharID)
		}
	}
}

func TestUnlockWhenNotLockedIsNoop(t *testing.T) {
	tr, now := newTestTrade(t)
	wantOK(t, tr.unlock(1, now))
	if tr.Rev != 1 {
		t.Fatalf("rev = %d, want 1", tr.Rev)
	}
}

func TestFailExchangeKeepsTradeEditing(t *testing.T) {
	tr, now := newTestTrade(t)
	wantOK(t, tr.setOffer(1, tradeOffer{Items: []int{5}, Pet: -1}, now))
	wantOK(t, tr.lock(1, tr.Rev, now, nil))
	wantOK(t, tr.lock(2, tr.Rev, now, nil))
	_, _ = tr.confirm(1, tr.Rev, now)
	tr.side(1).Offer.Items = nil // 失效的报价项先移除
	tr.failExchange(1, protocol.CodeBagFull, []int{5}, now)
	if tr.side(1).Locked || tr.side(2).Locked || tr.side(1).Confirmed {
		t.Fatal("failed exchange must return to editing")
	}
	if tr.LastErr == nil || tr.LastErr.Code != protocol.CodeBagFull || tr.LastErr.Who != 1 {
		t.Fatalf("last error = %+v", tr.LastErr)
	}
	// 之后的任何修改都会清掉 last_error
	wantOK(t, tr.setOffer(2, tradeOffer{Gold: 3, Pet: -1}, now))
	if tr.LastErr != nil {
		t.Fatal("last_error must be cleared by the next change")
	}
}

func TestIdleTimeout(t *testing.T) {
	tr, now := newTestTrade(t)
	if tr.expired(now.Add(tradeIdleTimeout - time.Second)) {
		t.Fatal("must not expire before the idle timeout")
	}
	if !tr.expired(now.Add(tradeIdleTimeout)) {
		t.Fatal("must expire at the idle timeout")
	}
	if got := tr.idleExpiresIn(now.Add(time.Minute)); got != 240 {
		t.Fatalf("idleExpiresIn = %d, want 240", got)
	}
	if got := tr.idleExpiresIn(now.Add(time.Hour)); got != 0 {
		t.Fatalf("idleExpiresIn after expiry = %d, want 0", got)
	}
}

func TestNonParticipantRejected(t *testing.T) {
	tr, now := newTestTrade(t)
	wantCode(t, tr.setOffer(9, tradeOffer{Pet: -1}, now), protocol.CodeTradeNotFound)
	wantCode(t, tr.unlock(9, now), protocol.CodeTradeNotFound)
	_, perr := tr.confirm(9, 1, now)
	wantCode(t, perr, protocol.CodeTradeNotFound)
}
