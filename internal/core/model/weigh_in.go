package model

// WeighIn is the Owner's weight for one week, Monday to Sunday in the Owner
// Time Zone. There is at most one per week; a later one replaces it.
type WeighIn struct {
	// Week is the Monday the week starts on.
	Week Day `bson:"_id"`
	// Kg is the weight to 1 decimal.
	Kg float64 `bson:"kg"`
}
