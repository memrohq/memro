package api

// One pass of the hourly jobs, for tests.
var (
	RecheckDomains = (*Server).recheckDomains
	AdvanceTrials  = (*Server).advanceTrials
)
