# REST version negotiation contract

Evidence reviewed: 2026-10-08.
Evidence level: official documentation and Tableau-maintained SDK source.
This record establishes the discovery and authentication contract; no live requests establish compatibility with a particular deployment.

## Discovery contract

[Server Info](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_server.htm) documents `GET /api/api-version/serverinfo`, available from REST API 2.4 onward.
The endpoint does not require authentication and returns HTTP 200.
Its XML response contains `tsResponse/serverInfo/restApiVersion`, with the supported REST version as text.
The adjacent `productVersion` element contains the product release as text and the build number in its `build` attribute.

[Tableau's SDK server implementation](https://github.com/tableau/server-client-python/blob/master/tableauserverclient/server/server.py) initializes the REST version to 2.4 before reading `server_info.get().rest_api_version`.
Its [Server Info implementation](https://github.com/tableau/server-client-python/blob/master/tableauserverclient/server/endpoint/server_info_endpoint.py) confirms unauthenticated discovery and the 2.4 floor.
The SDK spells the endpoint `serverInfo`; TADX uses the documented `serverinfo` spelling.
Together, these primary sources support using `/api/2.4/serverinfo` for discovery on older and newer servers.
TADX requests XML because REST 2.4 predates the JSON support introduced in REST 2.5.

## Authentication and version selection

[Official release history](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_whats_new.htm) introduces PAT sign-in in REST API 3.6, Tableau Server 2019.4.
The [version matrix](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_concepts_versions.htm) maps 2019.4 to REST 3.6 and 2026.2 to REST 3.29.
TADX's PAT-only authentication therefore establishes a minimum REST version of 3.6.
This minimum is an authentication requirement; individual capabilities retain their higher version requirements.

TADX discovers the server version before sending PAT credentials.
No baseline sign-in or authenticated discovery request is required.
The selected version is the numeric minimum of the reported server version and TADX's build maximum, currently 3.29.
The command retains the negotiated transport by canonical server and reuses it for sign-in and resource clients.
The version is not a user setting and is not persisted in configuration.
Legacy saved version fields are ignored and removed during the next configuration write.

Discovery uses the shared transport without a session and bounds the response to 64 KiB.
Missing, malformed, duplicated, or unsupported version evidence stops setup without an optimistic fallback.
Protocol failures preserve the shared HTTP status and request ID contract without rendering rejected response values.
The local command paths do not discover a server version.

## Verification limits

Hermetic recorded-response tests verify numeric negotiation, the authentication minimum, bounds, missing authorization headers, and shared transport errors.
Command composition tests separately establish one discovery per command and canonical server, version gate behavior, and discovery before sign-in.
These tests do not establish live deployment compatibility.
