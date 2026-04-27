package util

type Size string

const (
	Size1KB   Size = "1KB"
	Size10KB  Size = "10KB"
	Size100KB Size = "100KB"
	Size300KB Size = "300KB"
)

func (Size) Values() []Size {
	return []Size{
		Size1KB,
		Size10KB,
		Size100KB,
		Size300KB,
	}
}

func (s Size) String() string {
	return string(s)
}

func (s Size) Valid() bool {
	for _, v := range s.Values() {
		if s.String() == v.String() {
			return true
		}
	}

	return false
}
