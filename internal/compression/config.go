package compression

// Reduction levels
const (
	LevelOff        = "off"
	LevelLight      = "light"
	LevelModerate   = "moderate"
	LevelAggressive = "aggressive"
	LevelMaximum    = "maximum"
)

// Sentence scoring constants (from kreuzberg)
const (
	EdgePositionBonus      = 0.3
	IdealWordCountBonus    = 0.2
	NumericContentWeight   = 0.3
	CapsAcronymWeight      = 0.25
	LongWordWeight         = 0.2
	LongWordThreshold      = 8
	PunctuationDensWeight  = 0.15
	DiversityRatioWeight   = 0.15
	CharEntropyWeight      = 0.1
	SentenceKeepRatio      = 0.4
	IdealWordCountMin      = 3
	IdealWordCountMax      = 25
	ImportantWordMinLength = 10
)

// Semantic scoring constants
const (
	SemanticBaseImportance  = 0.3
	SemanticFilterThreshold = 0.3
	TechnicalTermMinLength  = 6
	ImportantCharMinAlpha   = 3
)
