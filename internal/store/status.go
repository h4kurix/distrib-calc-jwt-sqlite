package store

// Статусы выражений и задач. Значения совпадают с форматом API и БД.
const (
	StatusPending    = "pending"
	StatusInProgress = "in_progress"
	StatusClaimed    = "claimed"
	StatusCompleted  = "completed"
	StatusError      = "error"
)
