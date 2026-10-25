package model

import "time"

// Settings are the Owner's preferences that Core keeps: one document.
type Settings struct {
	// TimeZone is the Owner Time Zone, an IANA name such as "Asia/Tokyo".
	TimeZone string `bson:"timeZone"`
	// HeightCm is the Owner's height; 0 means not set yet.
	HeightCm        int             `bson:"heightCm"`
	WeighInReminder WeighInReminder `bson:"weighInReminder"`
}

// WeighInReminder is the weekly Notification reminding the Owner to weigh in.
type WeighInReminder struct {
	Enabled bool         `bson:"enabled"`
	Weekday time.Weekday `bson:"weekday"`
	// Time is the clock time in the Owner Time Zone, "HH:MM" on the hour or
	// half hour.
	Time string `bson:"time"`
}

// DefaultSettings are the settings until the Owner first saves theirs.
var DefaultSettings = Settings{
	TimeZone:        "UTC",
	WeighInReminder: WeighInReminder{Enabled: false, Weekday: time.Monday, Time: "08:00"},
}
