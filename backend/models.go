package main

import "time"

// 赛事
type Meet struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"not null" json:"name"`
	Venue     string    `json:"venue"`
	Date      string    `json:"date"`
	CreatedAt time.Time `json:"created_at"`
}

// 比赛项目（如 男子100米自由泳）
type Event struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	MeetID    uint      `gorm:"not null;index" json:"meet_id"`
	Name      string    `gorm:"not null" json:"name"`
	Distance  int       `json:"distance"`   // 米
	Stroke    string    `json:"stroke"`     // 泳姿
	Round     string    `json:"round"`      // 预赛/半决赛/决赛
	Status    string    `gorm:"not null;default:active" json:"status"` // active/swimoff/completed
	CreatedAt time.Time `json:"created_at"`
	Meet      Meet      `gorm:"foreignKey:MeetID" json:"meet,omitempty"`
}

// 泳道报名（一名运动员在某项目占一条泳道）
type Lane struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	EventID     uint   `gorm:"not null;uniqueIndex:idx_event_lane" json:"event_id"`
	LaneNo      int    `gorm:"uniqueIndex:idx_event_lane" json:"lane_no"`
	SwimmerName string `gorm:"not null" json:"swimmer_name"`
	Team        string `json:"team"`
}

// 电子计时设备原始读数（模拟接口写入，只追加，不覆盖）
type TimingReading struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	LaneID     uint      `gorm:"not null;index" json:"lane_id"`
	EventID    uint      `gorm:"not null;index" json:"event_id"`
	SplitIndex int       `gorm:"not null;default:0" json:"split_index"`        // 0=触壁总成绩, 1..n=分段
	SplitLabel string    `json:"split_label"`                                 // 50m / 100m ...
	TimeMS     *int64    `json:"time_ms"`                                     // 毫秒；nil 表示漏记
	Missed     bool      `gorm:"not null;default:false" json:"missed"`        // 设备判定漏记
	Source     string    `gorm:"not null;default:pad" json:"source"`          // pad(触板)/button(备用按钮)/watch
	DeviceRaw  string    `json:"device_raw"`                                  // 原始报文
	RecordedAt time.Time `json:"recorded_at"`
	CreatedAt  time.Time `json:"created_at"`
}

// 手记 / 补充材料：裁判与录入员均可提交，不直接改变名次
type ManualRecord struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	LaneID     uint      `gorm:"not null;index" json:"lane_id"`
	EventID    uint      `gorm:"not null;index" json:"event_id"`
	TimeMS     int64     `json:"time_ms"`
	SplitLabel string    `json:"split_label"`
	Note       string    `json:"note"`
	AuthorID   uint      `gorm:"not null" json:"author_id"`
	AuthorName string    `json:"author_name"`
	AuthorRole string    `json:"author_role"` // judge / clerk
	CreatedAt  time.Time `json:"created_at"`
}

// 复核案件：每条泳道一条，聚合争议、改判依据、总裁判裁定
type ReviewCase struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	EventID     uint       `gorm:"not null;uniqueIndex:idx_event_lane_case" json:"event_id"`
	LaneID      uint       `gorm:"not null;uniqueIndex:idx_event_lane_case" json:"lane_id"`
	Reason      string     `json:"reason"`       // 漏记 / 争议 / 并列
	Description string     `json:"description"`  // 改判依据
	Status      string     `gorm:"not null;default:open" json:"status"` // open / decided / withdrawn
	// 总裁判裁定
	Decision       string `json:"decision"`         // electronic / manual / swimoff / withdraw
	DecidedTimeMS  *int64 `json:"decided_time_ms"`  // 裁定采用的成绩（重赛结果或手动/电子）
	DecisionNote   string `json:"decision_note"`
	RefereeID      *uint  `json:"referee_id"`
	RefereeName    string `json:"referee_name"`
	DecidedAt      *time.Time `json:"decided_at"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// 榜单条目：每次发布生成一组新条目（快照，永不覆盖）
type RankingEntry struct {
	ID           uint   `gorm:"primaryKey" json:"id"`
	BoardID      uint   `gorm:"not null;index" json:"board_id"`
	EventID      uint   `gorm:"not null;index" json:"event_id"`
	Rank         int    `gorm:"not null" json:"rank"`
	DisplayRank  string `gorm:"not null" json:"display_rank"` // 1 / 2= / 3 ，并列标注
	LaneID       uint   `gorm:"not null" json:"lane_id"`
	LaneNo       int    `json:"lane_no"`
	SwimmerName  string `json:"swimmer_name"`
	Team         string `json:"team"`
	TimeMS       *int64 `json:"time_ms"`
	TimeText     string `json:"time_text"`
	Source       string `json:"source"`        // electronic / manual / swimoff
	SpecialState string `json:"special_state"` // tie / swimoff / withdrawn / ""
	Corrected    bool   `json:"corrected"`     // 相对上一版是否被更正
}

// 榜单版本
type LeaderboardVersion struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	EventID    uint       `gorm:"not null;index" json:"event_id"`
	Version    int        `gorm:"not null" json:"version"`
	ChangeNote string     `json:"change_note"`          // 更正原因
	Correction string     `json:"correction"`           // 更正摘要（人读）
	IsCurrent  bool       `gorm:"not null;default:false" json:"is_current"`
	PublishedBy uint      `json:"published_by"`
	PublisherName string   `json:"publisher_name"`
	CreatedAt  time.Time  `json:"created_at"`
	Entries    []RankingEntry `gorm:"foreignKey:BoardID" json:"entries"`
}
