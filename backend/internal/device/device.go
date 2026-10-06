// Package device 模拟电子计时触板设备的本地接口。
// 设备与成绩库完全独立：后端只能"拉取"它的读数并原样落库，不能改写设备数据。
package device

import (
	"fmt"
	"sync"
	"time"
)

type Packet struct {
	RaceCode    string    `json:"race_code"`
	LaneNo      int       `json:"lane_no"`
	SwimmerName string    `json:"swimmer_name"`
	DeviceID    string    `json:"device_id"`
	Status      string    `json:"status"` // OK / MISSED / PARTIAL
	Splits      []float64 `json:"splits"`
	FinishTime  *float64  `json:"finish_time"`
	Seq         int       `json:"seq"` // 设备流水号
	SentAt      time.Time `json:"sent_at"`
	Note        string    `json:"note,omitempty"`
}

type sessionState struct {
	base        []Packet
	retransmits map[int]Packet // laneNo -> 设备补发报文（漏记恢复）
	swimOff     []Packet
}

type Device struct {
	mu       sync.Mutex
	sessions map[string]*sessionState
	start    time.Time
}

func ptr(v float64) *float64 { return &v }

func New() *Device {
	d := &Device{sessions: map[string]*sessionState{}, start: time.Now()}
	d.sessions["M101"] = &sessionState{
		retransmits: map[int]Packet{},
		base: []Packet{
			{RaceCode: "M101", LaneNo: 1, SwimmerName: "陈一", DeviceID: "PAD-A01", Status: "OK", Splits: []float64{26.91}, FinishTime: ptr(55.84)},
			{RaceCode: "M101", LaneNo: 2, SwimmerName: "林二", DeviceID: "PAD-A02", Status: "OK", Splits: []float64{27.02}, FinishTime: ptr(56.10)},
			// 3 道触板漏记：只有 50 米分段，没有触壁时间
			{RaceCode: "M101", LaneNo: 3, SwimmerName: "黄三", DeviceID: "PAD-A03", Status: "MISSED", Splits: []float64{26.88}, FinishTime: nil, Note: "触板无响应"},
			{RaceCode: "M101", LaneNo: 4, SwimmerName: "周四", DeviceID: "PAD-A04", Status: "OK", Splits: []float64{26.40}, FinishTime: ptr(54.97)},
			// 5、6 道电子成绩完全相同 -> 并列
			{RaceCode: "M101", LaneNo: 5, SwimmerName: "吴五", DeviceID: "PAD-A05", Status: "OK", Splits: []float64{26.60}, FinishTime: ptr(55.32)},
			{RaceCode: "M101", LaneNo: 6, SwimmerName: "赵六", DeviceID: "PAD-A06", Status: "OK", Splits: []float64{26.61}, FinishTime: ptr(55.32)},
			{RaceCode: "M101", LaneNo: 7, SwimmerName: "钱七", DeviceID: "PAD-A07", Status: "OK", Splits: []float64{27.55}, FinishTime: ptr(57.01)},
			{RaceCode: "M101", LaneNo: 8, SwimmerName: "孙八", DeviceID: "PAD-A08", Status: "OK", Splits: []float64{27.40}, FinishTime: ptr(56.99)},
		},
	}
	d.sessions["M102"] = &sessionState{
		retransmits: map[int]Packet{},
		base: []Packet{
			{RaceCode: "M102", LaneNo: 1, SwimmerName: "黄三", DeviceID: "PAD-B01", Status: "OK", Splits: []float64{35.10, 73.40, 112.05}, FinishTime: ptr(150.22)},
			// 2 道电子成绩与裁判手记有明显出入 -> 成绩争议
			{RaceCode: "M102", LaneNo: 2, SwimmerName: "周四", DeviceID: "PAD-B02", Status: "OK", Splits: []float64{35.40, 74.02, 112.88}, FinishTime: ptr(151.50)},
			{RaceCode: "M102", LaneNo: 3, SwimmerName: "吴五", DeviceID: "PAD-B03", Status: "OK", Splits: []float64{34.88, 72.90, 111.40}, FinishTime: ptr(149.88)},
			// 4 道只有部分分段
			{RaceCode: "M102", LaneNo: 4, SwimmerName: "赵六", DeviceID: "PAD-B04", Status: "PARTIAL", Splits: []float64{35.60, 74.50}, FinishTime: nil, Note: "末端模块掉线"},
			{RaceCode: "M102", LaneNo: 5, SwimmerName: "钱七", DeviceID: "PAD-B05", Status: "OK", Splits: []float64{35.90, 75.10, 114.20}, FinishTime: ptr(153.10)},
			// 6 道与 1 道并列
			{RaceCode: "M102", LaneNo: 6, SwimmerName: "孙八", DeviceID: "PAD-B06", Status: "OK", Splits: []float64{35.12, 73.41, 112.06}, FinishTime: ptr(150.22)},
		},
	}
	return d
}

// Sessions 列出设备中可拉取的比赛场次
func (d *Device) Sessions() []map[string]interface{} {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := []map[string]interface{}{}
	for code, s := range d.sessions {
		missed, tied := 0, map[string]int{}
		times := map[float64]int{}
		for _, p := range s.base {
			if p.FinishTime == nil {
				missed++
			} else {
				times[*p.FinishTime]++
			}
		}
		for t, c := range times {
			if c > 1 {
				tied[fmt.Sprintf("%.2f", t)] = c
			}
		}
		out = append(out, map[string]interface{}{
			"race_code": code, "lanes": len(s.base), "missed_lanes": missed,
			"ties": tied, "retransmits": len(s.retransmits),
		})
	}
	return out
}

// Readings 返回当前报文：基础报文 + 设备补发报文（漏记恢复后覆盖该道报文，但历史报文在设备侧仍可追溯）
func (d *Device) Readings(code string) ([]Packet, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[code]
	if !ok {
		return nil, false
	}
	if len(s.swimOff) > 0 {
		return append([]Packet{}, s.swimOff...), true
	}
	out := make([]Packet, 0, len(s.base))
	for _, p := range s.base {
		if r, ok := s.retransmits[p.LaneNo]; ok {
			out = append(out, r)
		} else {
			out = append(out, p)
		}
	}
	for i := range out {
		out[i].SentAt = d.start
		out[i].Seq = i + 1
	}
	return out, true
}

// Retransmit 模拟设备补发某道报文（漏记恢复）。返回 false 表示场次/泳道不存在。
func (d *Device) Retransmit(code string, lane int, finish float64, splits []float64) (Packet, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[code]
	if !ok {
		return Packet{}, false
	}
	var base *Packet
	for i := range s.base {
		if s.base[i].LaneNo == lane {
			base = &s.base[i]
			break
		}
	}
	if base == nil {
		return Packet{}, false
	}
	p := *base
	p.Status = "OK"
	p.FinishTime = ptr(finish)
	if splits != nil {
		p.Splits = splits
	}
	p.Note = "备用计时模块补发"
	s.retransmits[lane] = p
	return p, true
}

// RegisterSwimOff 为重赛场次生成报文：按报名顺序拉开成绩，分出先后。
func (d *Device) RegisterSwimOff(code string, names []string, baseTimes []float64) []Packet {
	d.mu.Lock()
	defer d.mu.Unlock()
	gaps := []float64{-0.18, 0.16, 0.31}
	pk := make([]Packet, 0, len(names))
	for i, name := range names {
		t := baseTimes[i] + gaps[i%len(gaps)]
		pk = append(pk, Packet{
			RaceCode: code, LaneNo: i + 1, SwimmerName: name,
			DeviceID: fmt.Sprintf("PAD-S%02d", i+1), Status: "OK",
			Splits: []float64{float64(int(t/2*100)) / 100}, FinishTime: ptr(float64(int(t*100)) / 100),
			Note: "重赛", SentAt: d.start, Seq: i + 1,
		})
	}
	d.sessions[code] = &sessionState{base: pk, retransmits: map[int]Packet{}, swimOff: pk}
	return pk
}
