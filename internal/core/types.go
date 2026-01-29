package core

// Represents a location in a file (zero-based)
type Position struct {
	Line   uint // The line in the file (zero-based)
	Column uint // The column in the line (zero-based)
}

// Represents the type of a region
type RegionType int

const (
	RegionTypeJava            RegionType = iota // Type for a java region
	RegionTypePythonStatement                   // Type for a python statement region
	RegionTypePythonBlock                       // Type for a python code block region
)

// Represents a half-open [Start, end) region of a file
type Region struct {
	Type  RegionType // The type of data in this region
	Start Position   // The starting position of this region
	End   Position   // The ending position of this region
	Data  []byte     // The data in the region
}

// Represents a single translation unit (file)
type TranslationUnit struct {
	Data    []byte   // The data contained in the file
	Regions []Region // The mapped regions that comprise a file
}
