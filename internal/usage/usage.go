// Package usage は Claude サブスクリプションの使用量制限（5時間・7日間）の
// 使用率を記録・読み出しする。
//
// Claude Code は使用率を statusLine コマンドの stdin JSON（rate_limits）として
// 渡す。record-usage サブコマンドがそれを state ディレクトリに記録し、TUI は
// その記録を読む。
package usage

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// fileName は state ディレクトリ直下に置く記録ファイルの名前。
const fileName = "usage.json"

// Window は 1 つの制限枠（5時間 または 7日間）の使用状況。
type Window struct {
	UsedPercent float64   `json:"usedPercent"` // 0〜100
	ResetsAt    time.Time `json:"resetsAt"`
}

// IsExpired はリセット時刻を過ぎているかを返す。
func (w Window) IsExpired(now time.Time) bool {
	return !now.Before(w.ResetsAt)
}

// Limits は 5時間・7日間の各枠の使用状況。値が渡されなかった枠は nil になる。
type Limits struct {
	FiveHour *Window `json:"fiveHour,omitempty"`
	SevenDay *Window `json:"sevenDay,omitempty"`
}

// IsEmpty は枠が 1 つも無いかを返す。
func (l Limits) IsEmpty() bool {
	return l.FiveHour == nil && l.SevenDay == nil
}

// HasActiveWindow はリセット時刻を過ぎていない枠が 1 つでもあるかを返す。
func (l Limits) HasActiveWindow(now time.Time) bool {
	return (l.FiveHour != nil && !l.FiveHour.IsExpired(now)) ||
		(l.SevenDay != nil && !l.SevenDay.IsExpired(now))
}

// observedWindow は枠の値と、その値を初めて受け取った時刻。
type observedWindow struct {
	Window
	ObservedAt time.Time `json:"observedAt"`
}

// observe は前回記録した枠 prev に今回受け取った枠 incoming を反映する。
// 値が変わったときだけ観測時刻を進め、変わらなければ prev をそのまま返す。
// incoming が無い（リセット時刻を過ぎて Claude Code が枠を外した）場合も prev を残す。
//
// リセット時刻を過ぎた incoming も記録しない。観測時刻 now を付けると、他のセッションが
// 先に観測した次の枠より新しいとみなされ、Load で次の枠を隠してしまうため。
func observe(prev *observedWindow, incoming *Window, now time.Time) *observedWindow {
	if incoming == nil || incoming.IsExpired(now) {
		return prev
	}
	if prev != nil && prev.UsedPercent == incoming.UsedPercent && prev.ResetsAt.Equal(incoming.ResetsAt) {
		return prev
	}
	return &observedWindow{Window: *incoming, ObservedAt: now}
}

// sessionRecord は 1 セッションの statusLine から受け取った枠ごとの値。
type sessionRecord struct {
	FiveHour *observedWindow `json:"fiveHour,omitempty"`
	SevenDay *observedWindow `json:"sevenDay,omitempty"`
}

func (r sessionRecord) limits() Limits {
	return Limits{FiveHour: r.FiveHour.window(), SevenDay: r.SevenDay.window()}
}

// recordFile は記録ファイルの中身。キーは statusLine 入力の session_id。
type recordFile struct {
	Sessions map[string]sessionRecord `json:"sessions"`
}

// Record はセッション sessionID の statusLine が渡してきた値を記録する。
// 有効な枠が無い（API キー利用時など rate_limits が渡されない）ときや、
// 書き込みに失敗したときは何もしない。呼び出し元の record-usage は
// statusLine の表示を妨げないよう、エラーを返さず常に exit 0 で終了する方針のため。
//
// statusLine は API 応答以外のイベント（権限モードの切り替え、リセット時刻の
// 到達など）でも呼ばれ、そのセッションが最後に受け取った値を渡してくる。
// 止まっているセッションの古い値で新しい値を上書きしないよう、枠ごとに
// 値が変わったときだけ観測時刻を進める。
//
// 同時に書き込んだセッションの記録は片方が失われうるが、頻度が低く
// そのセッションの次の呼び出しで記録し直されるためロックは取らない。
func Record(stateDir, sessionID string, incoming Limits, now time.Time) {
	if stateDir == "" || !incoming.HasActiveWindow(now) {
		return
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return
	}

	f := loadRecordFile(os.DirFS(stateDir))
	prev := f.Sessions[sessionID]
	next := sessionRecord{
		FiveHour: observe(prev.FiveHour, incoming.FiveHour, now),
		SevenDay: observe(prev.SevenDay, incoming.SevenDay, now),
	}
	if next == prev {
		return
	}
	f.Sessions[sessionID] = next
	for id, r := range f.Sessions {
		if !r.limits().HasActiveWindow(now) {
			delete(f.Sessions, id) // セッションごとの記録がファイルに溜まり続けないよう、全枠がリセット済みのものを消す
		}
	}

	data, err := json.Marshal(f)
	if err != nil {
		return
	}
	writeFileAtomic(stateDir, fileName, data)
}

// writeFileAtomic は同一ディレクトリ内の一時ファイルに書いてから rename する。
// 読み取り側（TUI）が書きかけの不完全な JSON を見ないようにするため。
func writeFileAtomic(dir, name string, data []byte) {
	tmp, err := os.CreateTemp(dir, name+".*.tmp")
	if err != nil {
		return
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return
	}
	if err := os.Rename(tmpPath, filepath.Join(dir, name)); err != nil {
		os.Remove(tmpPath)
	}
}

// LastRecordedAt は記録ファイルの最終更新時刻を返す。記録が無ければ false。
// Record は値が変わったときだけ書き込むため、最後に値が変わった時刻になる。
func LastRecordedAt(stateDir string) (time.Time, bool) {
	if stateDir == "" {
		return time.Time{}, false
	}
	info, err := os.Stat(filepath.Join(stateDir, fileName))
	if err != nil || !info.Mode().IsRegular() {
		return time.Time{}, false
	}
	return info.ModTime(), true
}

// Load は記録ファイルを読み、枠ごとに最も新しく観測された値を返す。
// record-usage は任意（opt-in）の機能なので、ファイルが無い・壊れているといった
// 状況はエラーにせず空の Limits を返す。
func Load(fsys fs.FS) Limits {
	var fiveHour, sevenDay *observedWindow
	for _, r := range loadRecordFile(fsys).Sessions {
		fiveHour = newerObservation(fiveHour, r.FiveHour)
		sevenDay = newerObservation(sevenDay, r.SevenDay)
	}
	return Limits{FiveHour: fiveHour.window(), SevenDay: sevenDay.window()}
}

func newerObservation(a, b *observedWindow) *observedWindow {
	if a == nil || (b != nil && b.ObservedAt.After(a.ObservedAt)) {
		return b
	}
	return a
}

func (o *observedWindow) window() *Window {
	if o == nil {
		return nil
	}
	w := o.Window
	return &w
}

func loadRecordFile(fsys fs.FS) recordFile {
	f := recordFile{Sessions: make(map[string]sessionRecord)}
	raw, err := fs.ReadFile(fsys, fileName)
	if err != nil {
		return f
	}
	if err := json.Unmarshal(raw, &f); err != nil || f.Sessions == nil {
		return recordFile{Sessions: make(map[string]sessionRecord)}
	}
	return f
}
