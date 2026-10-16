// Package difficulty defines the four practice levels shared by listening
// and speaking modules. Labels are product defaults, not clinical classes.
package difficulty

const (
	Min = 1
	Max = 4
)

var labels = map[int]string{
	1: "Pemula",
	2: "Dasar",
	3: "Menengah",
	4: "Mahir",
}

func Valid(level int) bool { return level >= Min && level <= Max }

func Label(level int) string { return labels[level] }
