// Package care exposes opinionated application aggregates over EHR
// and Demographic resources: Patient, User, CaseLoad, CareTeam,
// Episode. Clinical writes go through Codec + composition endpoints;
// demographic writes go through PartyCodec + openehr/client/demographic.
package care
