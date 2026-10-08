package tokens

// BudgetBytesPerToken is the divisor behind Estimate. English prose runs near
// four bytes per token and dense code or non-Latin text near one to three, so
// three bytes per token overestimates typical input and never undercounts a CJK
// rune (three bytes, about one token).
//
// It is deliberately lower than EstimateBytesPerToken (3.94): that ratio labels
// a report and is measured on one prose file, while this one gates spend, where
// undercounting is the unsafe direction.
const BudgetBytesPerToken = 3

// Estimate approximates the token count of text from its UTF-8 length (one
// token per BudgetBytesPerToken bytes, rounded up). It is the one estimator for
// budget and cost decisions; it is conservative and is not a tokenizer. Use a
// Counter from New for numbers shown to a reader.
func Estimate(text string) int {
	if text == "" {
		return 0
	}
	return (len(text) + BudgetBytesPerToken - 1) / BudgetBytesPerToken
}
