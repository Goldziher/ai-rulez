package jsonmerge

// ClaimsOwner returns the predicate PlanElements wants for elements an earlier
// run recorded in claims. Each call consumes one recorded copy, so a hand-written
// duplicate of a claimed element is not mistaken for the claimed one.
func ClaimsOwner[T any](claims []Claim) func(T) bool {
	matchers := make([]*ElementMatcher, len(claims))
	for i, claim := range claims {
		matchers[i] = claim.NewElementMatcher()
	}
	return func(element T) bool {
		for _, matcher := range matchers {
			if matcher.Take(element) {
				return true
			}
		}
		return false
	}
}

// PlanElements decides an array in which ai-rulez owns only some elements. The
// array keeps the consumer's elements in their order and appends the elements of
// ours it lacks; an element an earlier run owned (owns reports it, once per owned
// copy) that ours no longer wants leaves, one copy per owned copy, so a
// hand-written duplicate stays. claimed is what this run added or already owned:
// an element identical to one the consumer wrote is theirs and is never claimed,
// so clean cannot take it back. Elements are compared by digest, which is how a
// manifest records them. Every writer of such an array decides through this one
// function so none can claim a hand-written duplicate.
func PlanElements[T any](owns func(T) bool, existing, ours []T) (value, claimed []T) {
	wanted := make(map[string]bool, len(ours))
	for _, element := range ours {
		wanted[Digest(element)] = true
	}
	present := make(map[string]bool, len(existing)+len(ours))
	retained := map[string]int{} // copies an earlier run owned that stay
	value = make([]T, 0, len(existing)+len(ours))
	for _, element := range existing {
		sum := Digest(element)
		owned := owns(element)
		if owned && !wanted[sum] {
			continue
		}
		value = append(value, element)
		present[sum] = true
		if owned {
			retained[sum]++
		}
	}
	for _, element := range ours {
		sum := Digest(element)
		switch {
		case !present[sum]:
			present[sum] = true
			value = append(value, element)
		case retained[sum] > 0:
			retained[sum]--
		default:
			continue // identical to an element the consumer wrote: theirs
		}
		claimed = append(claimed, element)
	}
	return value, claimed
}
