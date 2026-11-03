package character

// STAGE-276：隐身（docs/protocol.md 5.69）。隐身的角色不出现在别人的 players 与 players_changed 里，但他自己仍收到别人的变化。
// 判断依据是 playerEntry.sent：别人在上一次合并发送之后是否认为他在图上。窗口里的开关来回切换，只看窗口结束时的状态，
// 与窗口开始时相同就不推（先隐身又恢复，若数据变过则补一条 update）。
// 所有方法都在 x.mu 下调用（x.mu 是叶子锁，调用方可以持有 Presence.mu）。

// visibilityChanged 在隐身开关真的变了之后调用（调用时持有 p.mu，p.hidden 已经是新值）：同图的其他角色收到 leave 或 enter。
func (x *playerIndex) visibilityChanged(p *Presence) {
	if x == nil {
		return
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	e := x.entries[p.charID]
	if e == nil {
		return
	}
	shown := !p.hidden.Load()
	if shown == e.sent { // 和窗口开始时一样：撤掉窗口里相反的那一项，不推
		if cur := x.pendingOf(e.floor, p.charID); cur != nil && (cur.leave || cur.enter) {
			x.dropPending(e.floor, p.charID)
		}
		if shown && e.dirty {
			x.putPending(e.floor, &playerChange{charID: p.charID, name: e.name, entry: e})
		}
		return
	}
	if shown {
		x.putPending(e.floor, &playerChange{charID: p.charID, name: e.name, entry: e, enter: true})
	} else {
		x.putPending(e.floor, &playerChange{charID: p.charID, name: e.name, leave: true})
	}
}

// settleChanges 在合并窗口到期时（flush 持有 x.mu）整理一张地图的待发变化：
// 隐身者的 update 和 enter 不发（enter 的隐身者是真的不可见，协议 5.69）；同时更新别人的认知（sent、dirty）。
func (x *playerIndex) settleChanges(floor int, changes map[int64]*playerChange) {
	for id, c := range changes {
		if c.leave {
			if e := x.entries[id]; e != nil && e.floor == floor {
				e.sent = false
			}
			continue
		}
		if c.entry == nil {
			continue
		}
		if c.entry.p.hidden.Load() {
			delete(changes, id)
			continue
		}
		c.entry.dirty = false // enter 和 update 都是整条数据，发出去就不欠别人一条
		if c.enter {
			c.entry.sent = true
		}
	}
}

// pendingOf 返回地图上某个角色待发的变化（没有返回 nil）。调用时持有 x.mu。
func (x *playerIndex) pendingOf(floor int, charID int64) *playerChange {
	return x.pending[floor][charID]
}

// dropPending 撤掉地图上某个角色待发的变化。调用时持有 x.mu。
func (x *playerIndex) dropPending(floor int, charID int64) {
	m := x.pending[floor]
	if m == nil {
		return
	}
	delete(m, charID)
	if len(m) == 0 {
		delete(x.pending, floor)
	}
}
