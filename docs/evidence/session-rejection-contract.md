# Session rejection contract

Evidence reviewed: 2026-10-29.
Evidence level: official documentation and bounded live requests against one authorized disposable site at REST 3.29.
This record establishes how Tableau rejects a session that is no longer valid and why TADX can renew it and resend the rejected request.

## Session lifetime

[REST authentication](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_concepts_auth.htm) states that the credentials token is valid for 240 minutes by default, and Tableau Server administrators can change the idle limit.
[Personal access tokens](https://help.tableau.com/current/server/en-us/security_personal_access_tokens.htm) cannot hold concurrent sessions.
Signing in again with the same PAT, at the same site or another site, terminates the previous session, which then fails with an authentication error.

## Rejection responses

Live requests compared a token superseded by a later sign-in with the same PAT and a token Tableau never issued.
Both returned HTTP 401 from the classic REST API with error code 401002, summary "Unauthorized Access", and detail "Invalid authentication credentials were provided."
The current token kept working after a rejected request.
Only the classic REST API returned a Tableau error code.
Metadata GraphQL returned HTTP 401 with `{"status":401,"error":"Unauthorized"}`.
Content search returned HTTP 401 with error code `AUTHENTICATION_ERROR` in its JSON body.
Pulse returned HTTP 401 with `{"httpErrorCode":401,"message":"Unable to authenticate user"}`.
A sign-in failure uses HTTP 401 with REST code 401001, which is a different contract.

## Resending after renewal

A project create carrying a superseded token returned 401002, and an exact-name read afterward found no project.
Tableau therefore rejects the session before processing the request.
TADX treats HTTP 401 with code 401002, or with no Tableau code, as a session rejection, signs in again once, and resends the identical request.

A live run of the built CLI observed the saved-session epoch, which advances once per TADX sign-in with the PAT.
A second command reused the saved session without signing in.
After a sign-in outside TADX superseded the saved session, the next read, a parallel batch pull of three workbooks, and a project delete each renewed with exactly one sign-in and succeeded.
`tadx auth check` signed in each time, and the following command reused the checked session.

## Verification limits

Idle-timeout expiry was not observed live; it is expected to use the same token rejection, but that was not tested.
The no-effect observation covers a classic REST write; for Metadata, search, and Pulse, the live requests established the rejection shape only, and resending there relies on HTTP 401 meaning the request was not authenticated.
The live run cannot show whether the batch's rejected requests overlapped; hermetic race-detector tests cover concurrent rejections.
These requests establish one deployment's behavior, not every Tableau Server release.
