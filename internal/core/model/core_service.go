package model

import (
	"errors"
	"context"
	"time"
)

// CoreService is what other Modules' services may ask of Core. Modules read
// Core's data only through it, never from Core's collections.
type CoreService interface {
	// OwnerTimeZone is the time zone all of Nando's dates and clock times are in.
	OwnerTimeZone(ctx context.Context) (*time.Location, error)
	// Today is the current Day in the Owner Time Zone.
	Today(ctx context.Context) (Day, error)
	// Now is the current time on Core's clock, which tests set by hand.
	Now() time.Time
	// HeightCm is the Owner's height in cm, or 0 until it is set.
	HeightCm(ctx context.Context) (int, error)
	// MessengerFile downloads the file or photo with id that the Owner sent
	// assistant's bot, from Telegram. It returns ErrFileGone when Telegram
	// has no such file for that bot, or no longer keeps it.
	MessengerFile(ctx context.Context, assistant, id string) ([]byte, error)
	// ReadFile answers a file's data as messenger_get_file does: a PDF's
	// pages, or a CSV's lines, as pages and lines ask (both may be empty),
	// or an image for an image.
	ReadFile(ctx context.Context, data []byte, pages, lines string) (any, error)
}

// ErrFileGone is a file Telegram doesn't have for a bot: an unknown
// file_id, or one it no longer keeps.
var ErrFileGone = errors.New("Telegram has no such file for this bot")
