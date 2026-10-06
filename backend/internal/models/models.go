package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

// 角色
const (
	RoleClerk = "clerk" // 录入员：只能补充材料
	RoleJudge = "judge" // 裁判：可提交手记、发起复核
	RoleChief = "chief" // 总裁判：终局裁定 / 发布榜单
)

// 电子读数状态
const (
	ReadingOK      = "OK"      // 正常触壁
	ReadingMissed  = "MISSED"  // 漏记（无触壁时间）
	ReadingPartial = "PARTIAL" // 只有分段、缺触壁
)

// 泳道条目状态
const (
	EntryActive    = "ACTIVE"
	EntryWithdrawn = "WITHDRAWN"
)

// 复核案件状态
const (
	CaseOpen   = "OPEN"
	CaseClosed = "CLOSED"
)

// 复核原因
const (
	ReasonMissedTouch = "MISSED_TOUCH" // 漏记复核
	ReasonDispute     = "TIME_DISPUTE" // 成绩争议
	ReasonTie         = "TIE_RESOLVE"  // 并列处理（重赛）
	ReasonWithdrawal  = "WITHDRAWAL"   // 撤回
)

// 裁定结论
const (
	DecisionElectronic = "USE_ELECTRONIC" // 采用电子成绩
	DecisionManual     = "USE_MANUAL"     // 采用手动成绩
	DecisionSwimOff    = "USE_SWIMOFF"    // 采用重赛结果
)

// 榜单上成绩来源
const (
	SourceElectronic = "ELECTRONIC"
	SourceManual     = "MANUAL"
	SourceSwimOff    = "SWIMOFF"
	SourceNone       = "NONE" // 无有效成绩
)

// 榜单版本状态
const (
	BoardCurrent    = "CURRENT"
	BoardSuperseded = "SUPERSEDED"
)

// 榜单条目标记
const (
	FlagTie       = "TIE"       // 并列
	FlagSwimOff   = "SWIMOFF"   // 重赛决定
	FlagWithdrawn = "WITHDRAWN" // 撤回
	FlagMissed    = "MISSED"    // 漏记未决
)

// FloatSlice 以 JSON 文本落库的分段时间数组（单位：秒，保留两位小数）
type FloatSlice []float64

func (s FloatSlice) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	b, err := json.Marshal(s)
	return string(b), err
}

func (s *FloatSlice) Scan(v interface{}) error {
	if v == nil {
		*s = FloatSlice{}
		return nil
	}
	var b []byte
	switch t := v.(type) {
	case []byte:
		b = t
	case string:
		b = []byte(t)
	}
	if len(b) == 0 {
		*s = FloatSlice{}
		return nil
	}
	return json.Unmarshal(b, s)
}

type User struct {
	ID          uint   `gorm:"primarykey" json:"id"`
	Username    string `gorm:"uniqueIndex;size:32" json:"username"`
	PasswordSHA string `gorm:"size:64" json:"-"`
	Name        string `gorm:"size:32" json:"name"`
	Role        string `gorm:"size:16" json:"role"`
	Token       string `gorm:"uniqueIndex;size:64" json:"-"`
}

type Meet struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	Name      string    `gorm:"size:128" json:"name"`
	Venue     string    `gorm:"size:128" json:"venue"`
	Date      string    `gorm:"size:16" json:"date"`
	CreatedAt time.Time `json:"created_at"`
}

type Swimmer struct {
	ID   uint   `gorm:"primarykey" json:"id"`
	Name string `gorm:"size:32" json:"name"`
	Team string `gorm:"size:64" json:"team"`
}

type Race struct {
	ID           uint      `gorm:"primarykey" json:"id"`
	MeetID       uint      `gorm:"index" json:"meet_id"`
	Code         string    `gorm:"uniqueIndex;size:48" json:"code"`
	EventName    string    `gorm:"size:128" json:"event_name"`
	Distance     int       `json:"distance"`
	Stroke       string    `gorm:"size:32" json:"stroke"`
	Round        string    `gorm:"size:16" json:"round"`
	ParentRaceID uint      `gorm:"index;default:0" json:"parent_race_id"` // 重赛指向原项目
	CreatedAt    time.Time `json:"created_at"`
}

type LaneEntry struct {
	ID             uint      `gorm:"primarykey" json:"id"`
	RaceID         uint      `gorm:"uniqueIndex:idx_lane" json:"race_id"`
	LaneNo         int       `gorm:"uniqueIndex:idx_lane" json:"lane_no"`
	SwimmerID      uint      `json:"swimmer_id"`
	Status         string    `gorm:"size:16;default:ACTIVE" json:"status"`
	WithdrawReason string    `gorm:"size:256;default:''" json:"withdraw_reason"`
	Swimmer        Swimmer   `gorm:"foreignKey:SwimmerID" json:"swimmer"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// TimingReading 电子计时设备的原始读数。一经写入只允许追加批次，永不修改、不删除。
type TimingReading struct {
	ID          uint       `gorm:"primarykey" json:"id"`
	RaceID      uint       `gorm:"index" json:"race_id"`
	LaneEntryID uint       `gorm:"index" json:"lane_entry_id"`
	LaneNo      int        `json:"lane_no"`
	DeviceID    string     `gorm:"size:32" json:"device_id"`
	Status      string     `gorm:"size:16" json:"status"`
	Splits      FloatSlice `gorm:"type:text" json:"splits"`
	FinishTime  *float64   `json:"finish_time"`
	RawJSON     string     `gorm:"type:text" json:"raw_json"`
	ImportBatch int        `gorm:"index" json:"import_batch"`
	RecordedAt  time.Time  `json:"recorded_at"`
}

// ManualNote 手记与补充材料：录入员补材料，裁判提交手记及改判依据。
type ManualNote struct {
	ID           uint       `gorm:"primarykey" json:"id"`
	RaceID       uint       `gorm:"index" json:"race_id"`
	LaneEntryID  uint       `gorm:"index" json:"lane_entry_id"`
	AuthorID     uint       `json:"author_id"`
	AuthorName   string     `gorm:"size:32" json:"author_name"`
	AuthorRole   string     `gorm:"size:16" json:"author_role"`
	Content      string     `gorm:"type:text" json:"content"`
	ManualFinish *float64   `json:"manual_finish"`
	ManualSplits FloatSlice `gorm:"type:text" json:"manual_splits"`
	CreatedAt    time.Time  `json:"created_at"`
}

// ReviewCase 复核案件（漏记 / 争议 / 并列 / 撤回）
type ReviewCase struct {
	ID          uint       `gorm:"primarykey" json:"id"`
	RaceID      uint       `gorm:"index" json:"race_id"`
	LaneEntryID uint       `gorm:"index;default:0" json:"lane_entry_id"` // 0 表示整条泳道/项目级
	OpenedByID  uint       `json:"opened_by_id"`
	OpenerName  string     `gorm:"size:32" json:"opener_name"`
	ReasonType  string     `gorm:"size:24" json:"reason_type"`
	Summary     string     `gorm:"type:text" json:"summary"`
	Status      string     `gorm:"size:16;default:OPEN" json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	ClosedAt    *time.Time `json:"closed_at"`
	Rulings     []Ruling   `gorm:"foreignKey:CaseID" json:"rulings"`
}

// Ruling 总裁判裁定，只追加不改写。
type Ruling struct {
	ID            uint      `gorm:"primarykey" json:"id"`
	CaseID        uint      `gorm:"index" json:"case_id"`
	DecidedByID   uint      `json:"decided_by_id"`
	DeciderName   string    `gorm:"size:32" json:"decider_name"`
	Decision      string    `gorm:"size:24" json:"decision"`
	ManualFinish  *float64  `json:"manual_finish"` // USE_MANUAL 时可明确指定
	SwimoffRaceID uint      `gorm:"default:0" json:"swimoff_race_id"`
	Rationale     string    `gorm:"type:text" json:"rationale"`
	CreatedAt     time.Time `json:"created_at"`
}

// Board 榜单版本。发布新版只新增行，旧版置为 SUPERSEDED，永不覆盖。
type Board struct {
	ID               uint         `gorm:"primarykey" json:"id"`
	RaceID           uint         `gorm:"index" json:"race_id"`
	VersionNo        int          `gorm:"index" json:"version_no"`
	Status           string       `gorm:"size:16;default:CURRENT" json:"status"`
	CorrectionReason string       `gorm:"type:text;default:''" json:"correction_reason"`
	PublishedByID    uint         `json:"published_by_id"`
	PublisherName    string       `gorm:"size:32" json:"publisher_name"`
	CreatedAt        time.Time    `json:"created_at"`
	Entries          []BoardEntry `gorm:"foreignKey:BoardID" json:"entries"`
}

// BoardEntry 某一版榜单中一条泳道的快照
type BoardEntry struct {
	ID              uint     `gorm:"primarykey" json:"id"`
	BoardID         uint     `gorm:"index" json:"board_id"`
	LaneEntryID     uint     `json:"lane_entry_id"`
	LaneNo          int      `json:"lane_no"`
	SwimmerName     string   `gorm:"size:32" json:"swimmer_name"`
	Team            string   `gorm:"size:64" json:"team"`
	Rank            *int     `json:"rank"` // 并列同名次；无成绩/撤回为 nil
	ResolvedSeconds *float64 `json:"resolved_seconds"`
	DisplayTime     string   `gorm:"size:16" json:"display_time"`
	Source          string   `gorm:"size:16" json:"source"`
	Flags           string   `gorm:"size:64;default:''" json:"flags"` // TIE,SWIMOFF,WITHDRAWN,MISSED
	Note            string   `gorm:"size:256;default:''" json:"note"`
}

// AuditLog 审计：谁在什么岗位做了什么
type AuditLog struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	UserID    uint      `json:"user_id"`
	Name      string    `gorm:"size:32" json:"name"`
	Role      string    `gorm:"size:16" json:"role"`
	Action    string    `gorm:"size:64" json:"action"`
	Detail    string    `gorm:"type:text" json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}
