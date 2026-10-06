package main

import (
	"time"

	"gorm.io/gorm"
)

// seed 初始化演示账号与一组可直接演示的赛事数据（幂等）
func seed(db *gorm.DB) {
	var userCnt int64
	db.Model(&User{}).Count(&userCnt)
	if userCnt > 0 {
		return
	}

	users := []User{
		{Username: "chief", Password: "123456", Name: "周总裁", Role: "chief"},
		{Username: "judge", Password: "123456", Name: "吴裁判", Role: "judge"},
		{Username: "judge2", Password: "123456", Name: "郑裁判", Role: "judge"},
		{Username: "clerk", Password: "123456", Name: "王录入", Role: "clerk"},
		{Username: "device", Password: "123456", Name: "电子计时台", Role: "device"},
	}
	for i := range users {
		db.Create(&users[i])
	}

	meet := Meet{Name: "2026年城市游泳冠军赛", Venue: "市体育中心游泳馆", Date: "2026-10-06"}
	db.Create(&meet)

	ev1 := Event{MeetID: meet.ID, Name: "男子100米自由泳 决赛", Distance: 100, Stroke: "自由泳", Round: "决赛", Status: "active"}
	ev2 := Event{MeetID: meet.ID, Name: "女子50米蛙泳 决赛", Distance: 50, Stroke: "蛙泳", Round: "决赛", Status: "active"}
	ev3 := Event{MeetID: meet.ID, Name: "男子200米混合泳 决赛", Distance: 200, Stroke: "混合泳", Round: "决赛", Status: "active"}
	db.Create(&ev1)
	db.Create(&ev2)
	db.Create(&ev3)

	// 项目1：8条泳道，第4道漏记（用于漏记复核），第2、6道成绩相同（用于并列）
	lanes1 := []Lane{
		{EventID: ev1.ID, LaneNo: 1, SwimmerName: "陈一", Team: "海豚队"},
		{EventID: ev1.ID, LaneNo: 2, SwimmerName: "林二", Team: "蓝鲸队"},
		{EventID: ev1.ID, LaneNo: 3, SwimmerName: "张三", Team: "飞鱼队"},
		{EventID: ev1.ID, LaneNo: 4, SwimmerName: "李四", Team: "海豚队"},
		{EventID: ev1.ID, LaneNo: 5, SwimmerName: "王五", Team: "巨浪队"},
		{EventID: ev1.ID, LaneNo: 6, SwimmerName: "赵六", Team: "蓝鲸队"},
		{EventID: ev1.ID, LaneNo: 7, SwimmerName: "钱七", Team: "飞鱼队"},
		{EventID: ev1.ID, LaneNo: 8, SwimmerName: "孙八", Team: "巨浪队"},
	}
	for i := range lanes1 {
		db.Create(&lanes1[i])
	}
	now := time.Now()
	times := map[int]int64{
		1: 52350, // 52.35
		2: 51880, // 51.88 与第6道并列第1
		3: 53120, // 53.12
		// 第4道漏记，无触壁读数
		5: 52760, // 52.76
		6: 51880, // 51.88 并列
		7: 54010, // 54.01
		8: 55230, // 55.23
	}
	for _, l := range lanes1 {
		if t, ok := times[l.LaneNo]; ok {
			db.Create(&TimingReading{
				LaneID: l.ID, EventID: ev1.ID, SplitIndex: 1, SplitLabel: "50m",
				TimeMS: ptrInt64(t / 2), Source: "pad", DeviceRaw: "PAD-SPLIT-OK", RecordedAt: now.Add(-30 * time.Second),
			})
			db.Create(&TimingReading{
				LaneID: l.ID, EventID: ev1.ID, SplitIndex: 0, SplitLabel: "触壁",
				TimeMS: ptrInt64(t), Source: "pad", DeviceRaw: "PAD-TOUCH-OK", RecordedAt: now,
			})
		} else {
			// 第4道：分段有数据，触壁漏记
			db.Create(&TimingReading{
				LaneID: l.ID, EventID: ev1.ID, SplitIndex: 1, SplitLabel: "50m",
				TimeMS: ptrInt64(25670), Source: "pad", DeviceRaw: "PAD-SPLIT-OK", RecordedAt: now.Add(-28 * time.Second),
			})
			db.Create(&TimingReading{
				LaneID: l.ID, EventID: ev1.ID, SplitIndex: 0, SplitLabel: "触壁",
				TimeMS: nil, Missed: true, Source: "pad", DeviceRaw: "PAD-TOUCH-TIMEOUT", RecordedAt: now,
			})
		}
	}
	// 第4道自动立案（模拟 uploadReading 里的行为）
	missedLane := lanes1[3]
	db.Create(&ReviewCase{
		EventID: ev1.ID, LaneID: missedLane.ID, Reason: "电子计时漏记",
		Description: "电子计时设备未记录第4道(李四)触壁时间，原始报文: PAD-TOUCH-TIMEOUT，系统自动立案待复核",
		Status: "open",
	})
	// 裁判已在现场手记了第4道成绩
	db.Create(&ManualRecord{
		LaneID: missedLane.ID, EventID: ev1.ID, TimeMS: 52940, SplitLabel: "触壁",
		Note: "三表一致，手记52.94，运动员确已正常触壁，疑触板故障",
		AuthorID: users[1].ID, AuthorName: users[1].Name, AuthorRole: "judge",
	})

	// 项目2：4条泳道，数据完整（演示正常发布与历史版本）
	lanes2 := []Lane{
		{EventID: ev2.ID, LaneNo: 1, SwimmerName: "周晴", Team: "海豚队"},
		{EventID: ev2.ID, LaneNo: 2, SwimmerName: "吴雪", Team: "蓝鲸队"},
		{EventID: ev2.ID, LaneNo: 3, SwimmerName: "郑岚", Team: "飞鱼队"},
		{EventID: ev2.ID, LaneNo: 4, SwimmerName: "冯露", Team: "巨浪队"},
	}
	for i := range lanes2 {
		db.Create(&lanes2[i])
	}
	t2 := map[int]int64{1: 31220, 2: 30870, 3: 31550, 4: 32010}
	for _, l := range lanes2 {
		db.Create(&TimingReading{
			LaneID: l.ID, EventID: ev2.ID, SplitIndex: 0, SplitLabel: "触壁",
			TimeMS: ptrInt64(t2[l.LaneNo]), Source: "pad", DeviceRaw: "PAD-TOUCH-OK", RecordedAt: now,
		})
	}

	// 项目3：空项目，供手动编排演示
	_ = ev3
}

func ptrInt64(v int64) *int64 { return &v }
