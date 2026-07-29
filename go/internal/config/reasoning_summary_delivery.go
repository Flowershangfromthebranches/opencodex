package config

// ReasoningSummaryDeliveryValues are the delivery modes the Responses wire
// accepts (oracle: src/types.ts:1106-1111). An unknown value is rejected at
// config load rather than forwarded, because the upstream answers 400 and the
// user would see a failing turn instead of a bad setting.
var ReasoningSummaryDeliveryValues = []string{"sequential", "sequential_cutoff", "concurrent", "concurrent_cutoff"}

func ValidReasoningSummaryDelivery(value string) bool {
	for _, candidate := range ReasoningSummaryDeliveryValues {
		if value == candidate {
			return true
		}
	}
	return false
}
