package models

type GeositeCategory struct {
	ID          int64
	Tag, Action string
	Selected    bool
}
