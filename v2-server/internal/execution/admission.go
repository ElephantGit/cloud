package execution

// AdmissionState describes a confirmed admission outcome, independently of execution status.
// The zero value is unresolved, never evidence that work was not admitted.
type AdmissionState string

const (
	AdmissionAccepted    AdmissionState = "accepted"
	AdmissionNotAdmitted AdmissionState = "not_admitted"
)

// AdmissionResolution is returned only after SealAttempt has established an immutable outcome.
// AdmissionAccepted has the original, nonempty Ref, even if its turn has already ended.
// AdmissionNotAdmitted has a zero Ref and proves that this identity can never be admitted.
// No other State is a successful resolution. Any accompanying error invalidates the result.
// It is not a Workspace-release or execution-stop confirmation.
type AdmissionResolution struct {
	State AdmissionState
	Ref   Ref
}
