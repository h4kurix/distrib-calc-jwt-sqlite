package store

// rowScanner — минимальный контракт, общий для sql.Row и sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}
