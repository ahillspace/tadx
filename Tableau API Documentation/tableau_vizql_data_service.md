# Tableau VizQL Data Service - Complete Documentation

Scraped from the official VizQL Data Service documentation (https://help.tableau.com/current/api/vizql-data-service/en-us/).

An API to query Tableau published data sources.

## Contents

- [Introduction](#section-1)
- [What's New](#section-2)
- [Configuration](#section-3)
- [Using VDS with an Embedded Tableau Viz](#section-4)
- [Request Data Source Information](#section-5)
- [Request Data Source Model](#section-6)
- [Query a Data Source](#section-7)
- [Table Calculations Prerequisites](#section-8)
- [Table Calculations Overview](#section-9)
- [Table Calculation Query Examples](#section-10)
- [Custom Table Calculations](#section-11)
- [Nested Table Calculations](#section-12)
- [API Reference (all operations)](#section-13)
- [OpenAPI Schema](#section-14)
- [Error Codes](#section-15)
- [Limitations](#section-16)
- [Troubleshooting](#section-17)
- [Using VizQL Data Service with Postman](#section-18)

---

# VizQL Data Service Introduction

- How does it work?
- Endpoints
- Required tools
- Availability

The VizQL Data Service (VDS) provides a programmatic way for you to access your published data outside of a Tableau visualization. With a viz, you perform operations like dragging a pill to rows or columns. This accomplishes two things. It fetches data from the data source, and then creates a visualization of that data.

With VDS, you can fetch the data without any need for a visualization.

## How does it work?

VDS is a standard HTTP service with a Query data source method and a Request data source metadata method. In both methods, you describe your data request in the request body as a JSON object.

## Endpoints

The endpoint for the Query data source method is:

`POST https://{your-pod}.online.tableau.com/api/v1/vizql-data-service/query-datasource`

The endpoint for the Request data source metadata method is:

`POST https://{your-pod}.online.tableau.com/api/v1/vizql-data-service/read-metadata`

## Required tools

There are many ways to make API requests. You can create your own or use existing tools like cURL and Postman. See the [VizQL Data Service Collection](https://www.postman.com/salesforce-developers/salesforce-developers/folder/ydjw53q/vizql-data-service) in the Tableau APIs Postman collection to help get you started.

## Availability

VDS is available for both Tableau Cloud and Tableau Server. To use VDS with Tableau Server, you must be on Tableau Server 2025.1 or later.

---

# What's New

## VizQL Data Service 2026.2

*July 2026*

In this release, VizQL Data Service (VDS) has the following enhancements:

- Query workbook data sources. You can now query any data source that exists inside a workbook, which includes embedded data sources, or any published data source that the workbook is connected to. You make these queries in an interactive session, where you pass the session ID along with the query. See the Query data source, the Request data source metadata, and the Request data source model methods. For more information, see Find the workbook session ID and workbook data source ID, and Using VDS with an Embedded Viz.
- Support for [composable data sources](https://help.tableau.com/current/pro/desktop/en-us/datasource_extend_compose.htm), a Tableau feature that lets you build a data source out of other published data sources. In a composable data source, the data and fields are inherited from one or more published data sources. When a composable data source is published, its metadata returns an accurate and complete view of the data source, which guarantees that all VDS queries constructed at that moment execute as expected. However, if there are changes to the composable data source, such as when tables, fields, or captions are added or updated in the upstream sources, these changes aren’t automatically picked up by VDS. **To maintain stable and accurate VDS queries, you must always re-publish the latest version of your composable data source that is associated with the specific `datasourceLuid` that was used in the VDS query.**
- You can now request a new session to be used when you use one of the VizQL Data Service methods. You can set the `withNewSession` option for the `query-datasource`, `read-metadata`, `get-datasource-model` and `list-supported-functions` endpoints.
- **New Known Issues**:
    - When querying data through the VDS API with a categorical set filter, and a calculated field, the query returns empty results if the filter contains 10 or more string values. Queries with 9 or fewer values work correctly.
    - Queries to interactive workbook data sources have a 30 second timeout. If a query exceeds 30 seconds, it fails with a timeout error. This limitation will be removed in a future release.
    - Composable data sources that include two or more original published data sources, each with non-embedded credentials, are not supported. An original published data source is the parent, or genesis, of an extended or composable data source. If you provide credentials for multiple connections in a composable data source, the request fails with a 401 authentication error: “Federated data source contains unauthenticated connection.”

## VizQL Data Service 2026.1

*February 2026*

In this release, VizQL Data Service (VDS) has the following enhancements:

- **Result streaming:** VDS now streams back the results from a query request immediately, improving performance and lifting the previous 1-GB response size limit. For more information, see Results handling and error checking.
- **Server sent events (SSE):** Use the new optional `returnServerSentEvents` boolean field to stream results back using the standard server-sent event (SSE) protocol. See Server sent event support.
- **Row limits:** You can now specify a `rowLimit` within the `options` object to restrict the number of rows returned in a query.
- **Enhanced sorting:** The sorting algorithm now aligns with Tableau’s native sort, enabling more human-readable results for string-based numbers.
- **New metadata fields:** The Request data source metadata method now returns the following information:
    - **Group formulas:** If the data includes groups (that is, categorical bins), you can view what the groupings are when you specify the `includeGroupFormulas` option in the request.
    - **Aliases:** You can now see aliases for domain values of parameters and dimensions.
    - **Field types and roles:** The `read-metadata` response now includes the `fieldType` of each column, which can be NOMINAL, ORDINAL, or CONTINUOUS. NOMINAL and ORDINAL are subtypes of DISCRETE. The response also includes `fieldRole` of each column, which can be either MEASURE or DIMENSION.
    - **Hidden fields:** You can now show hidden fields using the `includeHiddenFields` option in the request.
    - **Decimal formatting:** The response now shows the number of decimal spaces set in the Number Format dialog box. The decimal formatting supports the following default number formats: `Number(Custom)`, `Currency(Custom)`, `Scientific`, `Percentage`, and `Custom`.
    - **Field descriptions:** Field descriptions now included in the metadata response. The field description is what appears under the Lineage tab, if you have Tableau Catalog. Tableau Catalog is available with Tableau Enterprise and Tableau+ licenses.
    - **Image role:** If a field has been assigned an image role (the field contains URLs that point to web images), this is shown in the metadata response.
    - **LOD indicator** The response now indicates when a calculation contains a level of detail (LOD) expression.
- **Groups in queries:** You can now query groups directly within both calculations and filters.
- **Logical table references:** Calculations now accept logical table names (for example, `COUNT([Addresses])`) directly. Note that the option `interpretFieldCaptionsAsFieldNames` still applies to logical tables, so when that option is set (`true`) the calculation, `COUNT([Addresses_200CC1380B5240B4B6592656B323D414]` is also valid and will return the same data.
- **Calculated fields:** You can now reference calculated fields as dimensions in table calculations.
- **Expanded filter support:** You can use aliases for members in a set and use those values as filters in a query (`"filterType": "SET"`). For more information, see Set filters.
- **List supported functions**: You can use the List supported Tableau functions method to return the list of Tableau functions supported for the specified data source. These are the functions that can be used when you create calculated fields in Tableau. The method returns the name of the function, the number and types of arguments, and the return type.

## VizQL Data Service 2025.3

*October 2025*

In this release, VizQL Data Service (VDS) has the following enhancements:

- **View updated metadata**: The Request data source metadata method now returns groups, bins, and parameters. You can also see the column class and calculation formulas. For more information, see the Example output section in Request Data Source Metadata.
- **Query existing bins**: You can now query bins and view the result. For more information, see the Parameters with bins example in Query a Data Source.
- **Override existing parameters**: You can now override the values currently persisted on the published data source. For more information, see the Calculated field with a parameter example in Query a Data Source.
- **Query new bins on the fly**: You can now create a new bin that does not already exist on the published data source. For more information, see the Create a new bin section in Creating Queries.
- **Query using fieldName instead of fieldCaption**: VDS now supports passing the `fieldName` in its methods. This allows renamed fields to be resolved correctly, preventing query failures. For more information, see the `interpretFieldCaptionsAsFieldNames` option in either the Request data source metadata, Request data source model or Query data source methods.
- **Expose type for all fields and formulas for calculations**: VDS now returns the current read metadata return value for a field to include parameters for `columnClass`, and `formula`. For more information, see the Request data source metadata response section in Request Data Source Metadata.
- **Query table calculations**: VDS supports everything you see in the quick table calculations, as well as custom table calculations and nested table calculations. For more information, see the Table Calculations section.
- **See the data source model**: You can now see the logical table and relationship metadata in a published data source. For more information, see Get Data Source Model.

## VizQL Data Service 2025.2

*June 2025*

This release has three enhancements: multiple credentials support, updates to the schema, and a Python client library.

### Support for multiple credentials

The VizQL Data Service now supports multiple credentials. For more information, see Data sources that require multiple credentials.

### Schema update

We updated the OpenAPI schema. The most significant change is that the Request data source metadata method now returns `defaultAggregation`. For more information, see the API documentation and the [OpenAPI schema](https://github.com/tableau/VizQL-Data-Service/blob/main/VizQLDataServiceOpenAPISchema.json).

### VizQL Data Service Python SDK

The VizQL Data Service Python SDK is a lightweight client library that enables interaction with Tableau’s VizQL Data Service APIs. It supports both Tableau Cloud and Tableau Server deployments and offers both synchronous and asynchronous methods for querying the APIs. For more information, see the [VizQL Data Service Python SDK](https://github.com/tableau/VizQL-Data-Service/blob/main/python_sdk/README.md) GitHub repo.

## VizQL Data Service 2025.1

*February 2025*

This is the initial public release of the VizQL Data Service (VDS). VDS provides a way for you to access your data outside of a Tableau visualization (viz).

With a viz, you perform operations like dragging a pill to rows or columns. This achieves two things: It fetches data from the data source, and it creates a visualization of that data. VDS allows you to perform a fetch of the data without the need for any visualization.

### Changes from the earlier release

There are substantial changes from the previous developer preview release. If you used the developer preview release, note these changes:

- We now refer to fields instead of columns. For more information, see the VDS API Documentation.
- Our authentication process has changed to use the Tableau REST API methods. For more information, see Configuration
- We now accept the data source LUID instead of the data source name, and have changed the syntax for specifying database credentials. For more information, see Connect to your data source.
- Quantitative filters used to handle both numbers and dates. This has been broken out into `QuantitativeNumericalFilter` and `QuantitativeDateFilter`, which are specific to their types.
- The `Filter` object has changed. Now each ‘`Filter` requires a `FilterField`.
- By default, a `Filter` excludes null values. Use an `includeNulls` field with a quantitative filter.
- We removed the `SPECIAL` filter type, and now a quantitative filter has new options of `ONLY_NULL`, and `ONLY_NON_NULL`, which you can use to filter appropriately.
- We updated the `RelativeDateFilter`. The `LASTN` `NEXTN` are used with `rangeN` to specify relative ranges. The other date range types are shortcuts. For more information, see the Relative date filters.
- We added a new `MatchFilter` type to do string matching.
- Some function have new and improved names.

## VizQL Data Service Developer Preview

*October 2024*

In this developer preview release, we added the API Access permission capability. To query a data source with the VizQL Data Service, you must assign this capability in the Permission dialog. For more information, see Assign API access capability in Configuration.

We also moved theVizQL Data Service Postman collection from its location in the Tableau pre-release site to its new [GitHub repository](https://github.com/tableau/VizQL-Data-Service-API-Postman-Collection).

*June 2024*

This is the initial developer preview release of the VizQL Data Service. This is a closed release, only available to a small number of developers.

This release introduces the initial set of API methods and endpoints for using the VizQL Data Service.

---

# Configuration

- Assign API access capability
- Workbook permissions
- Configure authentication
    - Sign in using a personal access token (PAT)
    - Sign in using a JSON web token (JWT)
    - Sign in using username and password
- Find the data source LUID
    - Option 1: Get the LUID using Tableau Cloud or Tableau Server
    - Option 2: Get the LUID using the Tableau REST API
- Find the workbook session ID and workbook data source ID
    - Get the session ID and data source ID using the Tableau Embedding API

## Assign API access capability

To query a data source with VizQL Data Service (VDS), you must first assign the **API Access** capability in the **Permission** dialog. For information about setting up this data source capability in the Tableau user interface, see the Permission Capabilities and Templates topic in either [Tableau Cloud Help](https://help.tableau.com/current/online/en-us/permissions_capabilities.htm#capabilities) or [Tableau Server Help](https://help.tableau.com/current/server/en-us/permissions_capabilities.htm#data-sources).

For information about setting up this data source capability using the REST API, see [Permissions](https://help.tableau.com/v0.0/api/rest_api/en-us/REST/rest_api_concepts_permissions.htm).

## Workbook permissions

To query a data source in a workbook with VDS, the following is required:

- The workbook must be published.
- The user must have **View**, **API Access**, and **Full Data Query** capabilities on the workbook.
- If the workbook data source is a published datasource, the user must have **View**, **API Access**, and **Connect** capabilities on the upstream data source.

For information about setting up workbook permissions, see the Permission Capabilities and Templates topic in either [Tableau Cloud Help](https://help.tableau.com/current/online/en-us/permissions_capabilities.htm#capabilities) or [Tableau Server Help](https://help.tableau.com/current/server/en-us/permissions_capabilities.htm#data-sources).

## Configure authentication

VDS requires that you send an authentication token with each request. The token lets Tableau Cloud or Tableau Server verify your identity and makes sure that you’re signed in. To get a token, you can call the Tableau REST API Sign In method, in one of three ways.

### Sign in using a personal access token (PAT)

To sign in using a PAT, see [Make a Sign In Request with a Personal Access Token](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_concepts_auth.htm#make-a-sign-in-request-with-a-personal-access-token) in the Tableau REST API Help for more information.

### Sign in using a JSON web token (JWT)

If you use a JWT, set the scope (scp) in the JWT to `tableau:viz_data_service:read`. The permissions of the user in the JWT determine query results.

See [Make a Sign In Request with a JWT](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_concepts_auth.htm#make-a-sign-in-request-with-jwt) in the Tableau REST API Help for more information on using a JWT to create a credentials token that you can use with VDS.

### Sign in using username and password

See [Make a Sign In Request with Username and Password](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_concepts_auth.htm#make-a-sign-in-request-with-username-and-password) in the Tableau REST API Help for more information.

For information about token expiration, changing the token timeout value, and more, see [Using the Authentication Token In Subsequent Calls](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_concepts_auth.htm#using_auth_token) in the Tableau REST API Help.

## Find the data source LUID

To run a VDS method, you must know the locally unique identifier (LUID) of the published data source you’re requesting information about. There are two options for finding the data source LUID.

### Option 1: Get the LUID using Tableau Cloud or Tableau Server

1. In the Tableau navigation menu, select **Explore**.
2. At the top of the **Explore** screen, select **All Data Sources** in the dropdown menu.
3. In the list of data sources, select the data source you want the LUID for.
4. On the data source page, select the **Details** icon () next to the data source name.

The LUID is at the bottom of the Data Source Details screen.

### Option 2: Get the LUID using the Tableau REST API

Use the [Query Data Sources](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_data_sources.htm#query_data_sources) method to return a list of data sources on your site. This method returns the official data source name in the `contentURL` attribute. The associated `id` of the `contentURL` is your data source LUID.

## Find the workbook session ID and workbook data source ID

A workbook data source is any data source that exists a inside a Tableau workbook. A workbook data source could include an embedded data source, or the published data source that the workbook is connected to. If the workbook is connected to a published data source, and the user makes edits to that published data source inside the workbook, those edits are available as part of the workbook data source.

To call a VDS method in an interactive session with a workbook on Tableau Server or Tableau Online, you must know the workbook session ID and the ID of the data source you’re requesting information about.

### Get the session ID and data source ID using the Tableau Embedding API

If you are embedding a Tableau Viz and also running queries using the VizQL Data Services API, you can programmatically get the session ID using the [Tableau Embedding API](https://help.tableau.com/current/api/embedding_api/en-us/index.html) method, `getVizQLDataServiceSessionInfo()`. For example, the following code snippet shows a method that takes a Tableau `viz` object and extracts the session ID (`vizqlServerSessionId`) and the global session header (`globalSessionHeader`). The global session header is needed if you are using Tableau Cloud. These values correspond to the `X-Session-Id` and the `Global-Session-Header` headers that you need to send when you query workbook data sources.

```
let vizqlServerSessionId = '';
let globalSessionHeader = '';

const getVizQLDataServiceSessionInfo = (viz) => {
    vizqlServerSessionId = viz.getVizQLDataServiceSessionInfo().vizqlServerSessionId;
    console.log('vizQLServerSessionId:' + viz.getVizQLDataServiceSessionInfo().vizqlServerSessionId);

    globalSessionHeader = viz.getVizQLDataServiceSessionInfo().globalSessionHeader;
    console.log('globalSessionHeader:' + viz.getVizQLDataServiceSessionInfo().globalSessionHeader);
  };
```

You can get the workbook data source IDs associated with the worksheet(s) in the viz. This example code snippet shows a method that gets the data source ID for a specific worksheet in a workbook. This workbook data source value is captured in the `datasourceInternalName` variable. This is the value that you use in a VDS call for `workbookDatasourceId`.

```
let datasourceInternalName = '';

const getDataSourceInternalName = async (viz) => {
  console.log(`Logging datasourceInternalName for worksheet for ${viz.workbook.name}...`);
  try {
    const salesSheet = viz.workbook.activeSheet.worksheets.find(sheet => sheet.name === 'Sale Map');
    const dataSources = await salesSheet.getDataSourcesAsync();
    datasourceInternalName = dataSources[0].id;
    console.log(`datasourceInternalName is:${datasourceInternalName}`);
  } catch (error) {
    console.log('Error fetching datasource:', error);
  }
};
```

After you’ve found the session ID, the global session header (Tableau Cloud only), and the workbook data source ID, you can call VDS. The following example shows how you include the session ID (`X-Session-Id`) and global session header (`Global-Session-Header`) in a JavaScript Fetch() API to query the workbook data source. The values `vizqlServerSessionId`, `globalSessionHeader`, and `datasourceInternalName` were obtained in the preceding Embedding API examples.

```
// set the VDS_BASE_PATH that corresponds to your Tableau instance
const VDS_QUERY_DATASOURCE = '/query-datasource';

async function callVizQLDataService() {

  try {

    const res = await fetch(VDS_QUERY_DATASOURCE, {
      method: 'POST',
      credentials: 'include',
      headers: {
        'Content-Type': 'application/json',
        'X-Session-Id': vizqlServerSessionId,
        'Global-Session-Header': globalSessionHeader
      },
      body: JSON.stringify({
        datasource: {
          workbookDatasourceId: datasourceInternalName,
        },
        query: {
          fields: [
            {
              fieldCaption: "Category"
            },
            {
              fieldCaption: "Sales",
              function: "SUM"
            }
          ]
        }
      }),
    });

      } catch (err) {
        console.error("Request failed.");

      }
    }
```

The following shows a complete listing of a web page that embeds a Tableau viz and extracts the session and workbook data source ID.

```html
<!DOCTYPE html>
<html>
<head>
  <title>Simple getSessionInfo</title>
  <script src="https://code.jquery.com/jquery-3.2.1.min.js"></script>
<script type="module" >

import { TableauEventType } from 'https:///javascripts/api/tableau.embedding.3.latest.js'

const tableauViz = document.getElementById('tableauViz');

let vizqlServerSessionId = '';
let globalSessionHeader = '';
let datasourceInternalName = '';

tableauViz.addEventListener(TableauEventType.FirstInteractive, async () => {
    getVizQLDataServiceSessionInfo(tableauViz);
    await getDataSourceInternalName(tableauViz);
}, { once: true });

const getVizQLDataServiceSessionInfo = (viz) => {
    vizqlServerSessionId = viz.getVizQLDataServiceSessionInfo().vizqlServerSessionId;
    console.log('vizQLServerSessionId:' + viz.getVizQLDataServiceSessionInfo().vizqlServerSessionId);

    globalSessionHeader = viz.getVizQLDataServiceSessionInfo().globalSessionHeader;
    console.log('globalSessionHeader:' + viz.getVizQLDataServiceSessionInfo().globalSessionHeader);
    };

const getDataSourceInternalName = async (viz) => {
  console.log(`Logging datasourceInternalName for worksheet for ${viz.workbook.name}...`);
  try {
    const salesSheet = viz.workbook.activeSheet.worksheets.find(sheet => sheet.name === 'Sale Map');
    const dataSources = await salesSheet.getDataSourcesAsync();
    datasourceInternalName = dataSources[0].id;
    console.log(`datasourceInternalName is:${datasourceInternalName}`);
  } catch (error) {
    console.log('Error fetching datasource:', error);
  }
};

</script>
</head>
<body>
  <tableau-viz id="tableauViz" src="{YOUR_SITE}/views/Superstore/Overview"></tableau-viz>
</body>
</html>
```

---

# Use VDS with an Embedded Tableau Viz

If you are building a web application that both embeds a Tableau viz and calls VizQL Data Service (VDS) from the same browser page, you need to configure your app so that the browser can attach the Tableau session cookie to both the embedded viz requests and the VDS API calls. This topic explains why that is challenging, and how to solve it using a reverse proxy, or a wildcard cookie domain (on Tableau Server only).

- The cookie problem
- Solutions for the cookie problem
- Set up reverse proxy for Tableau Cloud
- Set up reverse proxy for Tableau Server
- Set up a wildcard cookie domain on Tableau Server
- How the reverse proxy works
    - Reverse Proxy Requirements
    - Node.js backend
    - How the request flow works

## The cookie problem

When a user authenticates with Tableau—either by loading an embedded viz or by signing in directly—Tableau sets an `HttpOnly` session cookie in the browser (that is, the `workgroup_session_id` on Tableau Server and Tableau Cloud). This cookie:

- Cannot be read by JavaScript.
- Is sent automatically by the browser only to requests that match the cookie’s **domain scope**.
- Is inaccessible from a cross-origin iframe.

This means that if your web application lives on a different domain than Tableau, VDS calls from the browser will not include the session cookie, and authentication will fail.

## Solutions for the cookie problem

The `Domain` and `Path` attributes in the `Set-Cookie` header control which URLs receive a cookie. To get the embedded viz and VDS calls to share the Tableau session cookie, you configure your setup so that both run under the same cookie scope. The right approach depends upon your deployment.

The most reliable solution for Tableau Cloud and Tableau Server deployments is to set up a reverse proxy. This approach is recommended where the web app and Tableau are on different domains. For more information about this setup and the requirements, see How the reverse proxy works.

- Set up a reverse proxy for Tableau Server
- Set up a reverse proxy for Tableau Cloud

If you have a deployment where Tableau Server and the web app are on different hosts, but are in the same domain, you have another option. You can set the session cookie on a wildcard domain, which lets you avoid setting up a reverse proxy.

- Set up a wildcard cookie domain on Tableau Server
    - *(Note: cookie domain option is not available for Tableau Cloud)*

## Set up reverse proxy for Tableau Cloud

Setting up a reverse proxy is the most reliable way to enable the embedded viz and the VDS calls to share the same Tableau session cookie. This section shows an example setup that uses a Node.js backend deployed to Heroku, with the [Nginx buildpack](https://elements.heroku.com/buildpacks/heroku/heroku-buildpack-nginx) as a reverse proxy. See How the reverse proxy works for more information about reverse proxies.

#### Prerequisites

- A Tableau Cloud site
- A [Connected App](https://help.tableau.com/current/online/en-us/connected_apps.htm) configured on that site. See
- A Heroku account with the [Nginx buildpack](https://elements.heroku.com/buildpacks/heroku/heroku-buildpack-nginx) added
- Node.js

#### Step 1 — Assign API Access on your data source

1. In Tableau Cloud, open the data source you want to query.
2. Open the **Permissions** dialog and assign the **API Access** capability to the relevant users or groups.

For more information, see Assign API access capability.

#### Step 2 — Create a Connected App and generate a JWT

1. In Tableau Cloud, go to **Settings > Connected Apps** and create a new Connected App.
2. Note the **Client ID**, **Secret ID**, and **Secret Value**.
3. On your Node.js server, generate a JWT with the following claims:
    - `kid`: your Secret ID
    - `iss`: your Client ID
    - `sub`: the Tableau user’s email
    - `aud`: `tableau`
    - `scp`: `["tableau:views:embed", "tableau:viz_data_service:read"]`
    - `exp`: short expiry (5 minutes recommended)
4. Sign the JWT with your secret using HS256.

For more information on using a JWT with VDS, see Configure authentication.

#### Step 3 — Find the session ID and the workbook data source ID

For instructions, see Find the workbook session ID and workbook data source ID.

#### Step 4 — Build the Node.js backend

The Node.js server needs to handle two things:

1. **Serve `index.html`** with the embedded viz and a button to trigger the VDS query.
2. **Expose a VDS proxy endpoint** (for example, `POST /node/query`) that:
    - Receives the Tableau session cookie forwarded by Nginx from the browser.
    - Calls the VDS [Query data source](http://../reference/index.html#operation/QueryDatasource) method on Tableau Cloud along with the session headers, `X-Session-Id` and `Global-Session-Header`.
    - Returns the VDS response to the browser.

A minimal VDS request body looks like this:

```
{
  "datasource": {
    "workbookDatasourceId": "1a2a3b4b-5c6c-7d8d-9e0e-1f2f3a4a5b6b"
  },
  "query": {
    "fields": [
      { "fieldCaption": "State/Province" }
    ]
  }
}
```

For information about building queries, see Query a Data Source.

#### Step 5 — Configure Nginx as a reverse proxy

Create your Nginx configuration file (for example, `config/nginx.conf.erb`) with the following three routing rules:

```html
# Serve index.html and other static assets from Node.js
location = / {
    proxy_pass http://localhost:;
}

# Route VDS proxy calls to Node.js backend
location /node/ {
    proxy_pass http://localhost:;
}

# Proxy all other requests (viz, auth) to Tableau Cloud
location / {
    proxy_pass https://;
    proxy_set_header Host ;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
}
```

This ensures that the viz is loaded through your app domain, so the Tableau session cookie is scoped to your app domain. When the browser then calls `/node/query`, the same cookie is automatically attached.

#### Step 6 — Build the frontend

In the web app, in this example, `index.html`, embed the viz using the [Tableau Embedding API v3](https://help.tableau.com/current/api/embedding_api/en-us/index.html), pointing `src` at your **app domain and the viz path** (Nginx proxies it to Tableau Cloud). This web app also has a button that sends a VDS query to Tableau when clicked.

```
<tableau-viz
  src="https://views/YourWorkbook/YourView"
  token="">
</tableau-viz>

<!--  add a button to call VDS --->
<div id="vizQL">
  <h2>VizQL Data Service</h2>
  <button type="button" id="query-btn" disabled="true">Call VizQL Data Service</button>
  <pre id="results"></pre>
</div>
```

Add a button that calls your VDS proxy endpoint and displays the result. Include the `X-Session-Id` and `Global-Session-Header` in the call. For instructions on finding those values, see Find the workbook session ID and workbook data source ID

```
document.getElementById('query-btn').addEventListener('click', async () => {
  const response = await fetch('/node/query', {
    method: 'POST',
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      'X-Session-Id': vizqlServerSessionId,
      'Global-Session-Header': globalSessionHeader
    },
  });
  const data = await response.json();
  document.getElementById('results').textContent = JSON.stringify(data, null, 2);
});
```

**Note**: The `credentials: 'include'` option is required to ensure the browser includes cookies when calling its own origin.

#### Step 7 — Deploy and verify

1. Deploy the app to Heroku.
2. Open your app URL in a Chrome browser and open the Chrome DevTools.
3. Load the embedded viz. These instructions assume you have configured your web app as a connected app, and have generated and provided the JWT to authenticate and load the embedded viz.
4. Click the query button in the web app to call VDS.
5. In Chrome DevTools, open the **Network** tab, find the request to your `/node/query` endpoint, and inspect the **Cookie** header. The `workgroup_session_id` (or Tableau session) cookie domain should match your app domain. This confirms that cookie sharing is working correctly.

## Set up reverse proxy for Tableau Server

Setting up a reverse proxy is the most reliable way to enable the embedded viz and the VDS calls to share the same Tableau session cookie. This section shows an example setup that uses Nginx to place the Tableau Server, a web server (Node.js), and the VDS API calls all under one public hostname. All hosts must be within the same network. DNS configuration is managed by your administrator. See How the reverse proxy works for more information about reverse proxies.

#### Prerequisites

- Tableau Server (2025.1 or later)
- A web server running Node.js (hosting the embedding viz)
- An Nginx server, all within the same network (for example, an AWS private VPC)
- SSL certificates (self-signed or CA-signed; use the same certificate across all servers and the proxy)

#### Step 1 — Configure Tableau Server for the reverse proxy

Run the following TSM commands to configure Tableau Server to accept requests through the proxy (replace `analytics.example.com` with the name of your host):

```
tsm configuration set -k gateway.public.host -v "analytics.example.com"
tsm configuration set -k gateway.public.port -v 443
tsm configuration set -k gateway.trusted -v <ip-address-of-nginx-server>
tsm configuration set -k gateway.trusted_hosts -v "analytics.example.com"
tsm pending-changes apply
```

Configure SSL on Tableau Server using the same certificates as the proxy. For more information, see [Configure SSL for External HTTP Traffic to and from Tableau Server](https://help.tableau.com/current/server/en-us/ssl_config.htm) and [Configure Tableau Server to Work with a Reverse Proxy Server](https://help.tableau.com/current/server/en-us/proxy.htm).

#### Step 2 — Configure the Node.js web server

The Node.js server needs to:

1. Serve `index.html` with the embedded viz.
2. Expose an API endpoint (for example, `GET /api/query`) that reads the Tableau session cookie forwarded from the browser and uses it to call VDS on Tableau Server.

Make sure the Node.js host can resolve the hostname of the Tableau Server, either via an `/etc/hosts` entry or DNS lookup.

#### Step 3 — Configure Nginx

Configure Nginx at `analytics.example.com` with the following routing rules:

```
# Route all Tableau Server requests (viz, auth, REST API)
location / {
    proxy_pass https://<tableau-server-ip>;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
}

# Route the embedding app
location /app/ {
    proxy_pass https://<embedding-server-ip>/;
    proxy_set_header Host $host;
}

# Route VDS calls to the Node.js backend
location /app/api/ {
    proxy_pass https://<embedding-server-ip>/api/;
    proxy_set_header Host $host;
}
```

**Important**: Tableau Server sign-in redirects to `/sign_in`. Make sure your Nginx configuration includes a `location` entry that handles this redirect path. Missing this is a common configuration failure point.

#### Step 4 — Build the frontend

Make the VDS call using `credentials: 'include'` to ensure the browser sends the session cookie:

```
fetch('/app/api/query', { credentials: 'include' })
  .then(res => res.json())
  .then(data => console.log(data));
```

**Note:** On Tableau Server, you could also use `credentials: 'same-origin'` to ensure the browser sends the session cookie.

#### Step 5 — Verify

In Chrome DevTools, open the Network tab. Find the `/app/api/query` request and confirm that the **Cookie** header contains `workgroup_session_id` with domain `analytics.example.com`.

## Set up a wildcard cookie domain on Tableau Server

If your web app and Tableau Server are on different hosts, but in same domain, you can configure Tableau to set the session cookie on a wildcard domain. This allows all subdomains under a shared parent to receive the cookie automatically, without needing a reverse proxy for the back-end server.

Run the following TSM commands (replace `.example.com` with the parent domain of your network. Be sure to include the leading period (`.`)).

```
tsm configuration set -k vizportal.session_cookie_domain -v ".example.com"
tsm pending-changes apply
```

With this configuration, cookies set by `tableau.example.com` are automatically available to `webapp.example.com` and any other `*.example.com` subdomain. Some configurations that have been tested and confirmed to work:

| Web app server | Tableau Server | Cookie domain setting | Result |
|---|---|---|---|
| `test.example.com` | `analytics.example.com` | `.example.com` | Works |
| `test.analytics.example.com` | `analytics.example.com` | `analytics.example.com` | Works |
| `test.example.com` | `analytics.otherdomain.com` | `.example.com` | Does not work |

**Note**: This TSM setting is available for Tableau Server only. It is not available for Tableau Cloud.

## How the reverse proxy works

The most reliable solution for both Tableau Cloud and Tableau Server is to use a **reverse proxy**. The proxy makes all browser requests—the embedded viz and VDS API calls—appear to originate from a **single domain**. The proxy routes each request to the correct backend server. Because the browser sees only one domain, the Tableau session cookie set by the embedded viz is automatically included on subsequent VDS calls.

### Reverse Proxy Requirements

The proxy must route three types of requests:

- Embedded viz requests to Tableau Server/Cloud (`GET /views/...`, including login)
- VDS query requests, routed to the back-end server, which calls Tableau Server/Cloud directly (`POST /node/query`)
- Web app requests to the back-end server (`GET /`, serving the web app)

### Node.js backend

- A Node.js backend server must run behind the proxy.
- It makes calls to VDS using the `workgroup_session_id` cookie passed from the browser.
- The VDS call must originate from the Node.js backend, not directly from the browser.

### How the request flow works

---

# Request Data Source Metadata

- Get the data structure in the data source
- Request data source metadata response
    - Example responses

## Get the data structure in the data source

The Request data source metadata method provides you with information about the queryable fields in a data source. This method requires that you pass in the data source object, which could be the `datasourceLuid` for a published data source, or the `workbookDatasourceId` for a workbook data source. To find the `datasourceLuid`, see Find the data source LUID.

```
{
    "datasource": {
        "datasourceLuid": "1a2a3b4b-5c6c-7d8d-9e0e-1f2f3a4a5b6b",
    }
}
```

For a workbook data source, you must specify the `workbookDatasourceId`, and also include the session ID and the global session header (for Tableau Cloud) in the request headers. For instructions, see Find the workbook session ID and workbook data source ID.

```
{
    "datasource": {
        "workbookDatasourceId": "federated.1a2b3c4d5e6f7g8h9i1j2k3l4m5n"
    }
}
```

The request has the following options:

- `bypassMetadataCache`: Set to `true` if either the published data source or the underlying database has changed within your current session. When set to `true`, VizQL Data Service (VDS) refreshes the metadata. Note that VDS automatically invalidates the cache when the data source is republished. Use this option if your underlying database has changed.
- `withNewSession`: Set to `true` to request a new session. Clears the cached session for the user before a new session is created.
- `interpretFieldCaptionsAsFieldNames`: When set to `true`, the response returns the `fieldName` value everywhere the `fieldCaption` is used. This is also true for parameters. For example, if you set this field to `true`, the response returns, for example, `Parameter 2` instead of `Profit Bin Size`.
- `includeHiddenFields`: When set to `true`, the response shows hidden fields.
- `includeGroupFormulas`: When set to `true`, the response shows the group information for fields that are categorical bins.

## Request data source metadata response

This API method returns two objects: `data` and `extraData`.

The `data` object returns these fields:

- `fieldName`: The underlying field name on the data source.
- `fieldCaption`: The `fieldCaption` is what you must pass into a query. This is often the same as `fieldName`.
- `dataType`: Either `INTEGER`, `REAL`, `STRING`, `DATETIME`, `BOOLEAN`, `DATE`, `SPATIAL`, or `UNKNOWN`.
- `fieldRole`: Either `MEASURE` or `DIMENSION`.
- `fieldType`: Either `NOMINAL`, `ORDINAL`, or `CONTINUOUS`. `NOMINAL` and `ORDINAL` are subtypes of `DISCRETE`.
- `defaultAggregation`: The default aggregation applied to the field.
- `columnClass`: The type of field. Either `COLUMN`, `BIN`, `GROUP`, `CALCULATION`, or `TABLE_CALCULATION`.
- `formula`: The formula for this field if it’s a calculation.
- `groupFormula`: When `includeGroupFormulas` is set to `true` and the field is a categorical bin, the response shows group information (`groupFormula`). The group formula includes the `baseFieldName`, and the `groupings`, an array of objects defined by the group formula. Each group object includes the name of group (`alias`) and list of `members` that belong to the group. If `hasIncludeOther` is `true` (the **Include ‘Other’** option was selected in the Edit Group dialog box), all domain values not specified in a grouping will be grouped together. If set (`true`), the response could return a large amount of data. The default setting is `false`.
- `logicalTableId`: If you have a data model with more than one logical table, this tells you which table the field originated from.
- `description`: The description of the field (what appears under the Lineage tab for the data source) if you have enabled data catalog.
- `imageRole`: The value `URL` indicates whether the field is assigned an image role.
- `hidden`: Indicates whether the field is hidden (`true`) or not (`false`).
- `defaultFormatting`: Describes the default formatting for the field. The value of `decimalPlaces` indicates the number of decimal places used for the field.
- `isLODCalc`: Indicates whether the calculation contains a level of detail (LOD) expression.
- `aliases`: An array of aliases for fields. Each alias shows the original name (`member`) and it’s alias (`value`).

The `extraData` object returns a `parameters` object with these fields:

- `parameterType`: Either `ANY_VALUE` (accepts any value without restrictions), `LIST` (a set number of values from which you can choose), `QUANTITATIVE_DATE` (a date range with specified minimum, maximum, and granularity settings), `QUANTITATIVE_RANGE` (a numeric range with specified minimum, maximum, and step values), or `ALL` (no restrictions).
- `parameterName`: The internal name defined by Tableau.
- `parameterCaption`: The user-defined name of the parameter to identify and reference the parameter.
- `dataType`: Either `INTEGER`, `REAL`, `STRING`, `DATETIME`, `BOOLEAN`, and `DATE`, `SPATIAL`, `UNKNOWN`
- `value`: The default value for the parameter.
- `min`: The maximum value for the range.
- `max`: The minimum value for the range.
- `step`: The jumps between values.

- `DATETIME` and `SPATIAL` are currently not supported parameter `dataType` values.
- Date type parameters for using range controls don't support step size validation when configured with ISO Years, ISO Quarters, or ISO Weeks (all ISO period types).

### Example responses

#### A bin on the data source

```
{
    "data": [
        {
            "fieldName": "Profit (bin)",
            "fieldCaption": "Profit (bin)",
            "dataType": "INTEGER",
            "fieldRole": "DIMENSION",
            "fieldType": "ORDINAL",
            "defaultAggregation": "NONE",
            "columnClass": "BIN",
            "logicalTableId": "Orders_ECFCA1FB690A41FE803BC071773BA862",
            "formula": "SYS_NUMBIN([Profit],[Profit Bin Size])",
        }
    ]
}
```

#### A group on the data source

This example response shows the `groupForumula`, which is included when the `includeGroupForumulas` option is set to `true`. This example doesn’t show all the groupings that would be shown in the response, or all of the other fields.

```
{
    "data": [
        {
            "fieldName": "Product Name (group)",
            "fieldCaption": "Manufacturer",
            "dataType": "STRING",
            "fieldRole": "DIMENSION",
            "fieldType": "NOMINAL",
            "defaultAggregation": "COUNT",
            "columnClass": "GROUP",
            "groupFormula": {
                "baseFieldName": "Product Name",
                "groupings": [
                    {
                        "alias": "3D Systems",
                        "members": [
                            "3D Systems Cube Printer, 2nd Generation, Magenta",
                            "3D Systems Cube Printer, 2nd Generation, White"
                        ]
                    },
                    {
                        "alias": "3M",
                        "members": [
                            "3M Hangers With Command Adhesive",
                            "3M Office Air Cleaner",
                            "3M Organizer Strips",
                            "3M Polarizing Light Filter Sleeves",
                            "3M Polarizing Task Lamp with Clamp Arm, Light Gray",
                            "3M Replacement Filter for Office Air Cleaner for 20' x 33' Room"
                        ]
                    }
                ],
               "hasIncludeOther": true
            },
            "logicalTableId": "Orders_ECFCA1FB690A41FE803BC071773BA862",
            "hidden": false
        }
    ]
}
```

#### A calculation on the data source

```
{
    "data": [
        {
            "fieldName": "Calculation_1368249927221915648",
            "fieldCaption": "Profit Ratio",
            "dataType": "REAL",
            "fieldRole": "MEASURE",
            "fieldType": "CONTINUOUS",
            "defaultAggregation": "AGG",
            "columnClass": "CALCULATION",
            "isLODCalc": false,
            "formula": "SUM([Profit])/SUM([Sales])"
        }
    ]
}
```

#### Full example from the Superstore data source

```
{
    "data": [
        {
            "fieldName": "Category",
            "fieldCaption": "Category",
            "dataType": "STRING",
            "fieldRole": "DIMENSION",
            "fieldType": "NOMINAL",
            "defaultAggregation": "COUNT",
            "columnClass": "COLUMN",
            "logicalTableId": "Orders_ECFCA1FB690A41FE803BC071773BA862",
            "description": "This is the *Category*."
        },
        {
            "fieldName": "Discount",
            "fieldCaption": "Discount",
            "dataType": "REAL",
            "fieldRole": "MEASURE",
            "fieldType": "CONTINUOUS",
            "defaultAggregation": "SUM",
            "columnClass": "COLUMN",
            "logicalTableId": "Orders_ECFCA1FB690A41FE803BC071773BA862"
        },
        {
            "fieldName": "Regional Manager",
            "fieldCaption": "Regional Manager",
            "dataType": "STRING",
            "fieldRole": "UNKNOWN",
            "fieldType": "UNKNOWN",
            "defaultAggregation": "COUNT",
            "columnClass": "COLUMN",
            "logicalTableId": "People_D73023733B004CC1B3CB1ACF62F4A965"
        },
        {
            "fieldName": "Ship Date",
            "fieldCaption": "Ship Date",
            "dataType": "DATE",
            "fieldRole": "DIMENSION",
            "fieldType": "ORDINAL",
            "defaultAggregation": "YEAR",
            "columnClass": "COLUMN",
            "logicalTableId": "Orders_ECFCA1FB690A41FE803BC071773BA862"
        },
        {
            "fieldName": "Sub-Category",
            "fieldCaption": "Sub-Category",
            "dataType": "STRING",
            "fieldRole": "DIMENSION",
            "fieldType": "NOMINAL",
            "defaultAggregation": "COUNT",
            "columnClass": "COLUMN",
            "logicalTableId": "Orders_ECFCA1FB690A41FE803BC071773BA862"
        },
        {
            "fieldName": "Profit (bin)",
            "fieldCaption": "Profit (bin)",
            "dataType": "INTEGER",
            "fieldRole": "DIMENSION",
            "fieldType": "ORDINAL",
            "defaultAggregation": "NONE",
            "columnClass": "BIN",
            "logicalTableId": "Orders_ECFCA1FB690A41FE803BC071773BA862",
            "formula": "SYS_NUMBIN([Profit],[Profit Bin Size])"
        },
        {
            "fieldName": "Segment",
            "fieldCaption": "Segment",
            "dataType": "STRING",
            "fieldRole": "DIMENSION",
            "fieldType": "NOMINAL",
            "defaultAggregation": "COUNT",
            "columnClass": "COLUMN",
            "logicalTableId": "Orders_ECFCA1FB690A41FE803BC071773BA862"
        },
        {
            "fieldName": "Sales",
            "fieldCaption": "Sales",
            "dataType": "REAL",
            "fieldRole": "MEASURE",
            "fieldType": "CONTINUOUS",
            "defaultAggregation": "SUM",
            "columnClass": "COLUMN",
            "logicalTableId": "Orders_ECFCA1FB690A41FE803BC071773BA862"
        },
        {
            "fieldName": "Product Name (group)",
            "fieldCaption": "Manufacturer",
            "dataType": "STRING",
            "fieldRole": "DIMENSION",
            "fieldType": "NOMINAL",
            "defaultAggregation": "COUNT",
            "columnClass": "GROUP",
            "logicalTableId": "Orders_ECFCA1FB690A41FE803BC071773BA862"
        },
        {
            "fieldName": "Ship Mode",
            "fieldCaption": "Ship Mode",
            "dataType": "STRING",
            "fieldRole": "DIMENSION",
            "fieldType": "NOMINAL",
            "defaultAggregation": "COUNT",
            "columnClass": "COLUMN",
            "logicalTableId": "Orders_ECFCA1FB690A41FE803BC071773BA862"
        },
        {
            "fieldName": "Order Date",
            "fieldCaption": "Order Date",
            "dataType": "DATE",
            "fieldRole": "DIMENSION",
            "fieldType": "ORDINAL",
            "defaultAggregation": "YEAR",
            "columnClass": "COLUMN",
            "logicalTableId": "Orders_ECFCA1FB690A41FE803BC071773BA862"
        },
        {
            "fieldName": "Profit",
            "fieldCaption": "Profit",
            "dataType": "REAL",
            "fieldRole": "MEASURE",
            "fieldType": "CONTINUOUS",
            "defaultAggregation": "SUM",
            "columnClass": "COLUMN",
            "logicalTableId": "Orders_ECFCA1FB690A41FE803BC071773BA862"
        },
        {
            "fieldName": "Calculation_1368249927221915648",
            "fieldCaption": "Profit Ratio",
            "dataType": "REAL",
            "fieldRole": "MEASURE",
            "fieldType": "CONTINUOUS",
            "defaultAggregation": "AGG",
            "columnClass": "CALCULATION",
            "isLODCalc": false,
            "formula": "SUM([Profit])/SUM([Sales])"
        },
        {
            "fieldName": "Customer Name",
            "fieldCaption": "Customer Name",
            "dataType": "STRING",
            "fieldRole": "DIMENSION",
            "fieldType": "NOMINAL",
            "defaultAggregation": "COUNT",
            "columnClass": "COLUMN",
            "logicalTableId": "Orders_ECFCA1FB690A41FE803BC071773BA862"
        },
        {
            "fieldName": "Returned",
            "fieldCaption": "Returned",
            "dataType": "STRING",
            "fieldRole": "DIMENSION",
            "fieldType": "NOMINAL",
            "defaultAggregation": "COUNT",
            "columnClass": "COLUMN",
            "logicalTableId": "Returns_2AA0FE4D737A4F63970131D0E7480A03"
        },
        {
            "fieldName": "Postal Code",
            "fieldCaption": "Postal Code",
            "dataType": "STRING",
            "fieldRole": "DIMENSION",
            "fieldType": "ORDINAL",
            "defaultAggregation": "COUNT",
            "columnClass": "COLUMN",
            "logicalTableId": "Orders_ECFCA1FB690A41FE803BC071773BA862"
        },
        {
            "fieldName": "Order ID",
            "fieldCaption": "Order ID",
            "dataType": "STRING",
            "fieldRole": "DIMENSION",
            "fieldType": "NOMINAL",
            "defaultAggregation": "COUNT",
            "columnClass": "COLUMN",
            "logicalTableId": "Orders_ECFCA1FB690A41FE803BC071773BA862"
        },
        {
            "fieldName": "Product Name",
            "fieldCaption": "Product Name",
            "dataType": "STRING",
            "fieldRole": "DIMENSION",
            "fieldType": "NOMINAL",
            "defaultAggregation": "COUNT",
            "columnClass": "COLUMN",
            "logicalTableId": "Orders_ECFCA1FB690A41FE803BC071773BA862"
        },
        {
            "fieldName": "Quantity",
            "fieldCaption": "Quantity",
            "dataType": "INTEGER",
            "fieldRole": "MEASURE",
            "fieldType": "CONTINUOUS",
            "defaultAggregation": "SUM",
            "columnClass": "COLUMN",
            "logicalTableId": "Orders_ECFCA1FB690A41FE803BC071773BA862"
        },
        {
            "fieldName": "City",
            "fieldCaption": "City",
            "dataType": "STRING",
            "fieldRole": "DIMENSION",
            "fieldType": "NOMINAL",
            "defaultAggregation": "COUNT",
            "columnClass": "COLUMN",
            "logicalTableId": "Orders_ECFCA1FB690A41FE803BC071773BA862"
        },
        {
            "fieldName": "State/Province",
            "fieldCaption": "State/Province",
            "dataType": "STRING",
            "fieldRole": "DIMENSION",
            "fieldType": "NOMINAL",
            "defaultAggregation": "COUNT",
            "columnClass": "COLUMN",
            "logicalTableId": "Orders_ECFCA1FB690A41FE803BC071773BA862"
        },
        {
            "fieldName": "Region",
            "fieldCaption": "Region",
            "dataType": "STRING",
            "fieldRole": "DIMENSION",
            "fieldType": "NOMINAL",
            "defaultAggregation": "COUNT",
            "columnClass": "COLUMN",
            "logicalTableId": "Orders_ECFCA1FB690A41FE803BC071773BA862"
        },
        {
            "fieldName": "Country/Region",
            "fieldCaption": "Country/Region",
            "dataType": "STRING",
            "fieldRole": "DIMENSION",
            "fieldType": "NOMINAL",
            "defaultAggregation": "COUNT",
            "columnClass": "COLUMN",
            "logicalTableId": "Orders_ECFCA1FB690A41FE803BC071773BA862"
        }
    ],
    "extraData": {
        "parameters": [
            {
                "parameterType": "QUANTITATIVE_RANGE",
                "parameterName": "Parameter 1",
                "parameterCaption": "Top Customers",
                "dataType": "INTEGER",
                "value": 5.0,
                "min": 5.0,
                "max": 20.0,
                "step": 5.0
            },
            {
                "parameterType": "QUANTITATIVE_RANGE",
                "parameterName": "Parameter 2",
                "parameterCaption": "Profit Bin Size",
                "dataType": "INTEGER",
                "value": 200.0,
                "min": 50.0,
                "max": 200.0,
                "step": 50.0
            }
        ]
    }
}
```

---

# Get Data Source Model

- Get the data model in the data source
- Request data source model response
    - Example output

## Get the data model in the data source

The Request data source model method provides you with the logical table and relationship metadata in a published data source. This method requires that you pass in the data source object, which is the `datasourceLuid` for a published data source. To find the `datasourceLuid`, see Find the data source LUID.

```
{
    "datasource": {
        "datasourceLuid": "1a2a3b4b-5c6c-7d8d-9e0e-1f2f3a4a5b6b",
    }
}
```

```
workbookDatasourceId
```

```
datasource
```

```
get-datasource-model
```

The request has the following options:

- `bypassMetadataCache`: Set to `true` if either the published data source or the underlying database has changed within your current session. When set to `true`, VizQL Data Service (VDS) refreshes the metadata. Note that VDS automatically invalidates the cache when the data source is republished. Use this option if your underlying database has changed.
- `withNewSession`: Set to `true` to request a new session. Clears the cached session for the user before a new session is created.
- `interpretFieldCaptionsAsFieldNames`: When set to `true`, the response returns the `fieldName` value everywhere the `fieldCaption` is used. This is also true for parameters. For example, if you set this field to `true`, the response returns, for example, `Parameter 2` instead of `Profit Bin Size`.
- `includeHiddenFields`: When set to `true`, the response shows hidden fields.
- `includeGroupFormulas`: When set to `true`, the response shows the group information for fields that are categorical bins.

## Request data source model response

This API method returns two objects: `logicalTables` and `logicalTableRelationships`. The `logicalTables` object returns these fields:

- `logicalTableId`: The logical table LUID.
- `caption`: The user-defined logical table display name.

The `logicalTableRelationships` object returns these fields:

- `fromLogicalTable`: Contains the key:value pair for LUID of the table on the left.
- `toLogicalTable`: Contains the key:value pair for LUID of the table on the right.

### Example output

For this example, let’s use a data source with the following model in the Tableau user interface:

A request to this data source returns the following response:

```
{
    "logicalTables": [
        {
            "logicalTableId": "Orders_ECFCA1FB690A41FE803BC071773BA862",
            "caption": "Orders"
        },
        {
            "logicalTableId": "People_D73023733B004CC1B3CB1ACF62F4A965",
            "caption": "People"
        },
        {
            "logicalTableId": "Returns_2AA0FE4D737A4F63970131D0E7480A03",
            "caption": "Returns"
        }
    ],
    "logicalTableRelationships": [
        {
            "fromLogicalTable": {
                "logicalTableId": "Orders_ECFCA1FB690A41FE803BC071773BA862"
            },
            "toLogicalTable": {
                "logicalTableId": "People_D73023733B004CC1B3CB1ACF62F4A965"
            }
        },
        {
            "fromLogicalTable": {
                "logicalTableId": "Orders_ECFCA1FB690A41FE803BC071773BA862"
            },
            "toLogicalTable": {
                "logicalTableId": "Returns_2AA0FE4D737A4F63970131D0E7480A03"
            }
        }
    ]
}
```

For more information, see [The Tableau Data Model](https://help.tableau.com/current/online/en-us/datasource_datamodel.htm).

---

# Query a Data Source

- Anatomy of a query
    - The data source object
    - The query object
    - The options object
- Return format
    - OBJECTS return vs. ARRAYS return option
- Date formats
- Results handling and error checking
    - Streaming and Error checking
- Server-sent event support
    - SSE and error checking

## Anatomy of a query

The JavaScript Object Notation (JSON) request body for the Query data source method has three components.

- `datasource`: Required. This is an object that tells you which data source to query (a published data source or workbook data source) and, optionally, takes credentials. See The data source object.
- `query`: Required. This is how you specify which fields you want to retrieve information from. To get the names of the fields in your data source, see Get Data Source Information. For information about the structure of a query, see The query object.
- `options`: Optional. Options are metadata that you can use to adjust the behavior of the query. See The options object.

The following example shows the basic structure of a query.

```
{
"datasource": {
    "datasourceLuid": "",
 },
"options": {
    "debug": true
},
  "query": {
    "fields": [
       // fields here
    ]
  }
}
```

### The data source object

The `datasource` object takes a `datasourceLuid` for published data sources, or a `workbookDatasourceId` for a workbook data source. To find the `datasourceLuid`, see Find the data source LUID.

```
{
"datasource": {
    "datasourceLuid": "1a2a3b4b-5c6c-7d8d-9e0e-1f2f3a4a5b6b",
 },
  "query": {
    "fields": [
       // fields here
    ]
  }
```

For a workbook data source, you must specify the `workbookDatasourceId`, and also include the session ID and the global session header (for Tableau Cloud) in the request headers. For instructions, see Find the workbook session ID and workbook data source ID.

```
{
"datasource": {
    "workbookDatasourceId": "federated.1a2b3c4d5e6f7g8h9i1j2k3l4m5n",
 },
  "query": { // See below for more details
    "fields": [
       // Fields here
    ]
  }
}
```

#### Data sources that require credentials

If you have a data source that requires credentials, enter them in an additional connection object.

```
{
"datasource": {
    "datasourceLuid": "2bac64a3-e216-4d8f-891c-905f9ce33ac3",
    "connections": [
        {
          "connectionLuid": "31752d0a-ec9d-11ee-9ad5-0a61dcca52fb",
          "connectionUsername": "test",
          "connectionPassword": "password"
        }
    ]
 },
  "query": { // See below for more details
    "fields": [
       // Fields here
    ]
  }
}
```

For data sources that require non-embedded credentials, add the following fields to your connection object in the body of the request.

- `connectionUsername`
- `connectionPassword`

#### Data sources that require multiple credentials

If you have multiple connections that each require a username and password, add a list of connections. For each connection, you must provide a `connectionLuid`, a `connectionUsername`, and a `connectionPassword`.

To find the `connectionLuid`, use the [Query Data Source Connections](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_data_sources.htm#query_data_source_connections) method. This method returns an ID for each connection on the specified data source. The value of your connection’s ID is the value to use for `connectionLuid`.

```
{
"datasource": {
    "datasourceLuid": "2bac64a3-e216-4d8f-891c-905f9ce33ac3",
    "connections": [
        {
          "connectionLuid": "31752d0a-ec9d-11ee-9ad5-0a61dcca52fb",
          "connectionUsername": "test",
          "connectionPassword": "password"
        },
        {
          "connectionLuid": "54ec51f6-7b1c-40d8-a55c-d4a70333c358",
          "connectionUsername": "test2",
          "connectionPassword": "password2"
        }
    ]
 },
  "query": { // See below for more details
    "fields": [
       // Fields here
    ]
  }
}
```

### The query object

The `query` object contains these basic components:

- `fields`: Required. This contains an array of fields that define the output of the query. See Fields.
- `filters`: Optional. This contains an array of filters to apply to the query. They can include fields that aren’t in the fields array. See Filters.
- `parameters`: Optional. Parameters enable dynamic, user-defined inputs to control and customize the results returned by the data source query. They can act as variables (like numbers, dates, booleans, or strings) that replace constant values in calculations, or they can act as filters. See Parameters.

#### Fields

You can list the fields that you want from your published data source and see the returned data. Fields come in multiple forms: dimensions, measures, or custom calculations. You can sort fields, alias fields, and have their decimal places formatted (if they’re a number).

```
fieldCaption
```

```
fieldCaption
```

##### Dimensions

Pass in a `fieldCaption` and get the data for that field as a dimension.

The following example shows sorting, which is optional.

```
"fields": [
        {
            "fieldCaption": "Category",
            "sortPriority": 1
        },
        {
            "fieldCaption": "Sub-Category",
            "sortPriority": 2,
            "sortDirection": "DESC"
        },
]
```

##### Fields with aggregations or measures

You can add an aggregation, or function, to a field to treat the field as a measure.

```
"fields": [
        {
            "fieldCaption": "Sales",
            "function": "SUM",
            "maxDecimalPlaces": 2
        }
]
```

VDS supports the following functions for aggregations:

- `SUM`
- `AVG`
- `MEDIAN`
- `COUNT`
- `COUNTD`
- `MIN`
- `MAX`
- `STDEV`
- `VAR`
- `COLLECT`
- `YEAR`
- `QUARTER`
- `MONTH`
- `WEEK`
- `DAY`
- `TRUNC_YEAR`
- `TRUNC_QUARTER`
- `TRUNC_MONTH`
- `TRUNC_WEEK`
- `TRUNC_DAY`

##### Custom calculations

To specify a new, custom calculation as a field, give it a `fieldCaption` and a string in Tableau calculation syntax. To query an existing calculation, use the preceding methods to query an existing field.

```
"fields": [
        {
            "fieldCaption": "Profit Margin",
            "calculation": "SUM([Profit])/SUM([Sales])"
        }
]
```

The following is a complete list of things that you can add to your field object.

- `fieldCaption`: Required. A name for the column must be provided. Either a reference to a specific column in the data source or, in the case of a calculation, a user-supplied name for the calculation.
- `fieldAlias`: Optional. An alternate name to give the column in the output. This is only used in Object format output.
- `function`: Optional. Provide a Function for a Measure to generate an aggregation against that Field’s values. For example, providing the SUM Function will cause an aggregated SUM to be calculated for that Field. A Field cannot contain both a Function and a Calculation.
- `calculation`: Optional. Provide a Calculation to generate a new data Field based on that Calculation. The Calculation should contain a string based on the Tableau Calculated Field Syntax. Since this is a newly generated Field, you must give it its own unique `fieldCaption`. A Field can’t contain both a Function and a Calculation.
- `maxDecimalPlaces`: Optional. The maximum number of decimal places in the returned value. Any trailing zeros (0s) will be dropped. The `maxDecimalPlaces` value must be greater or equal to 0.
- `sortDirection`: Optional. The direction of the sort, either ascending or descending.
- `sortPriority`: Optional. To enable sorting on a specific Field, provide a `sortPriority` for that Field, and that Field will be sorted. The `sortPriority` provides a ranking of how to sort Fields when multiple Fields are being sorted. The highest priority (lowest number) field is sorted first. If only one field is being sorted, then any value may be used for sortPriority. The value should be an integer and can be negative.

##### Additional rules about fields

- You must query at least one field.
- You can’t query the same field twice.
- You can’t have duplicate sort priorities.

#### Filters

You can also filter the fields you receive back from the data source. To specify a filter, you can create a field (like above) to operate on. This field doesn’t need to be in the list of original fields. There are different kinds of filters, each requiring a `filterType` and a field to filter on.

##### Set filters

You can include or exclude certain values from your dataset when showing results. The following filter is used for dimensions. You must set the Boolean `exclude` and provide a list of values to either exclude (when `exclude=true`) or include (when `exclude=false`). If you’ve created aliases for members in a set, you can use those aliases in the query. For example, suppose that you added “Home” as an alias for “Home Office.” You could use “Home” as the value in the filter.

```
"filters": [
    {
      "field": {
         "fieldCaption": "Segment"
      },
      "filterType": "SET",
      "values": [ "Home", "Consumer"],
      "exclude": false
    }
]
```

Note that the `values` array can’t be empty.

##### Quantitative filters

A quantitative filter operates on a field with an aggregation. The filter can specify a minimum, maximum, or range of values. The filter can also be used to handle both null and non-null values.

Quantitative filters must have `quantitativeFilterType`, which can be one of the following (with some rules):

- `MIN`: The “min” value must be set.
- `MAX`: The “max” value must be set.
- `RANGE`: Both the “min” and “max” values must be set.
- `ONLY_NULL`: Show only null values in the return data set. That is, don’t include a “min” value or a “max” value.
- `ONLY_NON_NULL`: Show only non-null values. That is, don’t include a “min” value or a “max” value.

If you have type `ONLY_NULL` or `ONLY_NON_NULL`, you can’t have minimums and maximums specified.

If you have type `MIN`, `MAX`, or `RANGE`, you can also set the additional property `includeNulls` to true (it’s false by default) if you’d like to include nulls.

There are two types of quantitative filters, one for dates and one for numerical values.

###### Quantitative numerical filters

For measures, set the `filterType` to `QUANTITATIVE_NUMERICAL`.

```
// measure range example
"filters": [
  {
    "column": {
        "fieldCaption": "Sales",
        "function": "SUM"
    },
    "filterType": "QUANTITATIVE_NUMERICAL",
    "quantitativeFilterType": "MIN",
    "min": 10000
  }
]
```

###### Quantitative date filters

For dates, set the `filterType` to `QUANTITATIVE_DATE`.

The only difference here’s that, instead of specifying min and max, you can instead specify `minDate` and `maxDate`.

```
// date range example
"filters": [
  {
     "field": {
         "fieldCaption": "Order Date"
      },
    "filterType": "QUANTITATIVE_DATE",
    "quantitativeFilterType": "MAX",
    "maxDate": "2020-04-01"
  }
]
```

##### Relative date filter

A relative date filter is a way to specify a range of dates from a given anchor. If you don’t set an anchor, today’s date will be used by default.

You must also set the variables `periodType` and `dateRangeType`.

`periodType` can be one of the following values:

- `MINUTES`
- `HOURS`
- `DAYS`
- `WEEKS`
- `MONTHS`
- `QUARTERS`
- `YEARS`

`dateRangeType` can be one of the following values:

- `LAST`
- `CURRENT`
- `NEXT`
- `LASTN` (requires you to add `rangeN` field)
- `NEXTN` (requires you to add `rangeN` field)
- `TODATE`

See the following Tableau **Filter[Order Date]** dialog box to help understand what these `dateRangeTypes` mean.

1. `LAST`
2. `LASTN`
3. `CURRENT`
4. `NEXTN`
5. `NEXT`
6. `TODATE`

###### Some examples

```
// January 1st, 2020 - March 1st, 2020
"filters": [
  {
    "filterType": "DATE",
    "field": {
    	"fieldCaption": "Order Date"
     },
    "periodType": "MONTHS",
    "dateRangeType": "NEXTN",
    "rangeN": 3,
    "anchorDate": "2020-01-01"
  }
]
```

```
// January 1st, 2020 - December 31st, 2020
"filters": [
  {
    "filterType": "DATE",
    "field": {
    	"fieldCaption": "Order Date"
     },
    "periodType": "YEARS",
    "dateRangeType": "CURRENT",
    "anchorDate": "2020-01-01"
  }
]
```

```
// All dates up to today's date (no anchor defaults to today)
"filters": [
  {
    "filterType": "DATE",
     "field": {
    	"fieldCaption": "Order Date"
     },
    "periodType": "YEARS",
    "dateRangeType": "TODATE,
  }
]
```

Requirement: You must have a `periodType` and a `dateRangeType`. If you specify a `rangeN`, then your `dateRangeType` must be `lastN` or `nextN`.

##### Top N filter

You can see top or bottom values of a given field by some aggregation.

A top N filter, or `filterType`: `TOP`, allows you to find the top N results of a given category. You must specify the following inputs:

- `fieldCaption`: The same as other queries, this is the column on which you want to filter.
- `fieldToMeasure`: This is a filter column on which you’re finding the top or bottom results of.
- `direction`: Either `TOP` or `BOTTOM` to show the highest or lowest results.
- `howMany`: An integer for how many results you would like to see.

See the following example, “Top 10 States with the highest Profit.”

```
"filters": [
  {
    "field": {
    	"fieldCaption": "State/Province"
     },
    "filterType": "TOP",
    "howMany": 10,
    "fieldToMeasure": {
      "fieldCaption": "Profit",
      "function": "SUM"
    },
    "direction": "TOP"
  }
]
```

Requirement: You must have `howMany` and `fieldToMeasure`.

##### Match filter

You can also provide substrings to conditionally match various filter types.

```
"filters": [
    {
      "field": {
    	"fieldCaption": "State/Province"
      },
      "filterType": "MATCH",
      "startsWith": "A",
      "endsWith": "a",
      "contains": "o",
      "exclude": false
    }
]
```

Requirement: You must have at least one of `startsWith`, `endsWith`, or `contains`.

##### Context filter

You can add a context filter and create a dependent filter on a quantitative filter or a top N filter, ideally to improve performance. You can add “context” as a boolean field to any filter to make it a context filter. For more information about Tableau context filters, see [Use Context Filters](https://help.tableau.com/current/pro/desktop/en-us/filtering_context.htm).

```
"filters": [
    {
        "field": {
    	    "fieldCaption": "SubCategory"
        },
        "filterType": "TOP",
        "howMany": 10,
        "fieldToMeasure": {
            "fieldCaption": "Sales",
            "function": "SUM"
        },
       "direction": "TOP"
    },
    {
        "field": {
    	    "fieldCaption": "Category"
        },
        "filterType": "SET",
        "values": [ "Furniture"],
        "exclude": false,
        "context": true
    }
]
```

##### Some additional rules about filters

- Set Filters, Match Filters, and Relative Date Filters can’t have functions or calculations.
- You can’t have multiple filters for a single field.

#### Parameters

When you query data sources, you can include parameters in the query request payload to override existing default parameter values. You can use parameters in:

- **Calculated fields**: Parameters enable dynamic formulas using [Parameter Caption/Name] syntax, allowing users to create flexible calculations that respond to parameter value changes.
- **Ad-hoc calculations**: Parameters can be referenced in temporary calculations created directly and providing a quick way to test parameter-driven logic without creating formal calculated fields.
- **Bins**: Parameters can control bin sizes and ranges dynamically, allowing users to adjust data grouping intervals interactively without recreating bin fields.

##### Parameters with ad hoc calculations

Parameters can be used within ad hoc calculated fields to create dynamic calculations directly in the data source query. Ad-hoc calculations are temporary calculations that you create by adding a calculated field. This lets you test parameter-driven logic quickly. To use parameters in ad hoc calculations, reference them using the syntax `[Parameter Caption/Name]` within your calculation expression.

For example, you can create calculations like `INT([Profit] / [Profit Bin Size]) * [Profit Bin Size]` directly. This approach is particularly useful for testing parameter logic, experimenting with what-if scenarios, and rapid prototyping. For more information, see [Ad Hoc Calculations](https://help.tableau.com/current/pro/desktop/en-us/calculations_calculatedfields_adhoc.htm).

```
"fields": [
	{
		"fieldCaption": "Binned Profit",
		"calculation": "INT([Profit] / [Profit Bin Size]) * [Profit Bin Size]"
	}
]
```

##### Bin with a hard-coded bin size

To query a bin on Sales with a static hard-coded bin size of 10:

```
"query": {
    "fields": [
        {
            "fieldCaption": "Sales",
            "function": "COUNT"
        },
       // bin size is hard coded as 10 on the published data source, no way to change it here
        {
            "fieldCaption": "Sales (bin)",
            "sortPriority": 1
        }
    ]
  }
```

The response:

```
{
    "data": [
        {
            "COUNT(Sales)": 1398,
            "Sales (bin)": 0
        },
        {
            "COUNT(Sales)": 1528,
            "Sales (bin)": 10
        },
        {
            "COUNT(Sales)": 858,
            "Sales (bin)": 20
        },
        {
            "COUNT(Sales)": 690,
            "Sales (bin)": 30
        },
        {
            "COUNT(Sales)": 492,
            "Sales (bin)": 40
        },
        {
            "COUNT(Sales)": 349,
            "Sales (bin)": 50
        },
        {
            "COUNT(Sales)": 337,
            "Sales (bin)": 60
        },
... etc. ...
    ]
}
```

#### Bin with a parameter bin size

This example queries Profit (bin). This bin has a dynamic bin size, which is a parameter. We know from our metadata request that the default value of the bin size is 200. In this example, we change the bin size to 50.

Note: Because of the rules around what **Profit Bin Size** can be (as seen in the metadata), the bin size can only be between 50 and 200 with a step size of 50. Eventually VDS throws an error if you try to pass in a value that is not allowed.

If you try the query without using a parameter:

```
"query": {
    "fields": [
        {
            "fieldCaption": "Profit",
            "function": "COUNT"
        },{
            "fieldCaption": "Profit (bin)", // default value of bin size is 200
            "sortPriority": 1
        }
    ]
  }
```

The response:

```
{
    "data": [
        {
            "COUNT(Profit)": 1,
            "Profit (bin)": -33
        },
        {
            "COUNT(Profit)": 1,
            "Profit (bin)": -20
        },
        {
            "COUNT(Profit)": 1,
            "Profit (bin)": -19
        },
        {
            "COUNT(Profit)": 1,
            "Profit (bin)": -17
        },
... etc. ...
    ]
}
```

The same query, but you have overridden the bin size to be a different value:

```
"query": {
    "fields": [
        {
            "fieldCaption": "Profit",
            "function": "COUNT"
        },{
            "fieldCaption": "Profit (bin)", // We are overriding bin size below
            "sortPriority": 1
        }
    ],
    "parameters": [
        {
            "parameterCaption": "Profit Bin Size",
            "value": 50 // Override value from 200 to 50
        }
    ]
  }
```

The response:

```
{
    "data": [
        {
            "COUNT(Profit)": 1,
            "Profit (bin)": -132
        },
        {
            "COUNT(Profit)": 1,
            "Profit (bin)": -77
        },
        {
            "COUNT(Profit)": 1,
            "Profit (bin)": -75
        },
        {
            "COUNT(Profit)": 1,
            "Profit (bin)": -68
        },
        {
            "COUNT(Profit)": 1,
            "Profit (bin)": -59
        },
... etc. ...
    ]
}
```

#### Calculated field with a parameter

This example queries a calculation that uses a parameter.

The following query doesn’t override the parameter:

```
"query": {
    "fields": [
        {
            "fieldCaption": "User Greeting"
        }
    ]
}
```

The return shows the default greeting:

```
{
    "data": [
        {
            "User Greeting": "Hello, test!"
        }
    ]
}
```

This next query uses a parameter to override the value of the Greeting String:

```
"query": {
    "fields": [
        {
            "fieldCaption": "User Greeting"
        }
    ],
    "parameters": [
        {
            "parameterCaption": "Greeting String",
            "value": "Hi" // Override default with "Hi"
        }
    ]
  }
```

The response shows the new value:

```
{
    "data": [
        {
            "User Greeting": "Hi, test!"
        }
    ]
}
```

##### Create a New bin

To make a new bin on a field, give the measure field and the bin size. VDS validates that the field you provide a bin on is a measure.

For example, this query:

```
"query": {
    "fields": [
        {
            "fieldCaption": "Discount", // Create a new bin on the field "Discount"
            "binSize": 0.1,
            "sortPriority": 1
        },
        {
            "fieldCaption": "Discount",
            "function": "COUNT"
        }
    ]

  }
```

returns:

```
{
    "data": [
        {
            "Discount (bin)": 0.0,
            "COUNT(Discount)": 4925
        },
        {
            "Discount (bin)": 0.1,
            "COUNT(Discount)": 148
        },
        {
            "Discount (bin)": 0.2,
            "COUNT(Discount)": 3936
        },
        {
            "Discount (bin)": 0.3,
            "COUNT(Discount)": 27
        },
... etc. ...
    ]
}
```

### The options object

In addition to the `datasource` object and the `query` object, VDS lets you use the following additional options that can adjust the behavior of your query.

- `debug`: (Boolean) Returns more detailed error messages from VDS in debug mode.
- `bypassMetadataCache`: Set to `true` if either the published data source or the underlying database has changed within your current session. When set to `true`, VizQL Data Service (VDS) refreshes the metadata.
- `withNewSession`: Set to `true` to request a new session. Clears the cached session for the user before a new session is created.
- `interpretFieldCaptionsAsFieldNames`: (Boolean) When set to `true`, you can use the `fieldName` value everywhere the `fieldCaption` is used. This includes query fields, filter fields, fields to measure in a Top filter, parameters, table calculations, on the fly calculations, and any other place where you can pass in a `fieldCaption`. For example, if you set this field to `true`, you could query for `Parameter 2` instead of `Profit Bin Size`.
- `includeHiddenFields`: When set to `true`, the response shows hidden fields.
- `includeGroupFormulas`: When set to `true`, the response shows group information for fields that are categorical bins.
- `disaggregate`: (Boolean) Determines whether to aggregate results. This is the equivalent of Tableau web authoring UI. For help, see [Disaggregate Data](https://community.tableau.com/s/question/0D54T00000C603oSAB/disaggregate-data). This is only available for Query data source.
- `returnFormat`: Whether the return format is OBJECTS (human-readable) or ARRAYS (compact).
- `rowLimit`: Restricts the number of rows returned in the results of a query. This option takes an integer value of `>=1`. Use this option to reduce the number of rows and eliminate some clutter. While this option limits the number of rows shown in results of the query, VDS still must retrieve all the results from the underlying database. This option won’t necessarily improve performance or provide faster results.
- `returnServerSentEvents`: When set to `true`, the results are returned using the server-sent events (SSE) protocol. If not specified, or if the option is set to `false`, the results are streamed back as JSON. For more information, see Results handling and error checking and Server-sent event support.

Example query with options

```
{
"datasource": { // See above for more details
	// datasource info here
 },
"query": { // See above for more details
  "fields": [
       // Fields here
  ]
 },
"options": {
   "returnFormat": "OBJECTS",
   "debug": true,
   "disaggregate": false
}
}
```

#### Query using fieldName value

In the example of a calculation on the data source, the response shows both the `fieldName` and the `fieldCaption`:

```
{
    "data": [
        {
            "fieldName": "Calculation_1368249927221915648",
            "fieldCaption": "Profit Ratio",
            "dataType": "REAL",
            "defaultAggregation": "AGG",
            "columnClass": "CALCULATION",
            "formula": "SUM([Profit])/SUM([Sales])"
        }
    ]
}
```

If you set `"interpretFieldCaptionsAsFieldNames": true,`, you can use the `fieldName` value as the value for `fieldCaption` in your query. In this case, your query would use `"fieldCaption":"Calculation_1368249927221915648"`.

## Return format

VDS always returns the response body in JSON.

### OBJECTS return vs. ARRAYS return option

The `OBJECTS` option returns field names as human-readable JSON objects. The `ARRAYS` option returns lists of data values.

```
// OBJECTS return
{
    "data": [
        {
            "Ship Mode": "Second Class",
            "SUM(Sales)": 466671.11140000017
        },
        {
            "Ship Mode": "Standard Class",
            "SUM(Sales)": 1378840.5509999855
        },
        {
            "Ship Mode": "Same Day",
            "SUM(Sales)": 129271.955
        },
        {
            "Ship Mode": "First Class",
            "SUM(Sales)": 351750.73690000066
        }
    ]
}

// ARRAYS return
{
    "data": [
        [
            "Second Class",
            466671.11140000017
        ],
        [
            "Standard Class",
            1378840.5509999855
        ],
        [
            "Same Day",
            129271.955
        ],
        [
            "First Class",
            351750.73690000066
        ]
    ]
}
```

## Date formats

- VDS does not support datetimes. You can only use dates.
- VDS uses the [RFC 3339 standard](https://www.ietf.org/rfc/rfc3339.txt) to input dates.
- VDS outputs dates and time in the RFC 3339 standard.
- VDS doesn’t support time zones in dates.

For more information, see the VizQL Data Service API documentation.

## Results handling and error checking

Starting in Tableau 2026.1, the Query data source method streams results. Instead of waiting for the full query to complete and assembling the entire response first, as was previously the case, VDS begins returning results when they’re produced. This change typically improves the time that it takes to see results, and improves the overall query performance.

Starting in Tableau 2026.1, there’s no longer a 1-GB limit on the response size of a query. Because of this change, you could potentially get back a very large response. You should consider taking steps to handle this situation. In addition, the Query data source method has a 30-minute timeout. Queries that take longer than 30 minutes, even if they are in the process of returning results, cause a timeout error and cause the query to fail. The timeout error returns an HTTP status of 408, and VDS status of 408801.

### Streaming and Error checking

Because the results are streamed, errors are returned differently depending on when they occur:

- If an error occurs before any results are streamed, the response uses the normal REST pattern: a non-200 HTTP status code and a response body containing a `TableauError` object. For example, an invalid request is rejected, and the query is never run. Here’s an example of an error that occurred when there was an invalid query: ` { "errorCode": "400803", "message": "Unknown Field: Stae/Province.", "datetime": "2025-10-30T20:53:13.804356606Z" } `
- If an error occurs after streaming response has started, the HTTP status code and headers have already been sent and can’t be changed. In this case, you might still receive HTTP 200 even though the query request failed. When the error occurs during streaming, the data stops being added, an `error` property is appended to the results, and the response is completed. The `error` property contains a `TableauError` object (which includes a VDS error code. The first three digits of the error code correspond to the HTTP status). Because an error can occur even when the HTTP status indicates success, check both the HTTP status code and whether an `error` property is present in the response body. Here’s an example that shows an error encountered as the results are streamed back: ` { "data": [ { "Category": "Furniture", "Sub-Category": "Bookcases", "Country/Region": "Canada", "Product Name": "Atlantic Metals Mobile 2-Shelf Bookcases, Custom Colors", "AVG(Sales)": 72.294 }, { "Category": "Furniture", "Sub-Category": "Bookcases", "Country/Region": "Canada", "Product Name": "Bush Westfield Collection Bookcases, Fully Assembled", "AVG(Sales)": 168.31 } ], "error": { "errorCode": "500000", "datetime": "2025-10-30T22:30:08.054791Z", "debug": null } } `

## Server-sent event support

Starting in Tableau 2026.1, VDS supports streaming the response using the standard [Server Sent Events](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events/) protocol.

To enable server-sent events (SSE), set the `returnServerSentEvents` option when you call the query data source method. The option takes a boolean value and is false by default. When the option is false, VDS streams the results to the client as JSON as described in Results handling and error checking. If true, VDS streams the results to the client using the server-sent events protocol (`text/event-stream`). From that protocol, VDS only uses the data property. Each data property encodes in JSON the `SseResultStream` as specified the VDS OpenAPI specification.

Here’s an example query request that sets the `returnServerSentEvents` option:

```
{
  "datasource": {
    "datasourceLuid": ""
  },
  "options": {
    "returnServerSentEvents": true
  },
  "query": {
        "fields": [
        {
        "fieldCaption": "State/Province",
        "sortDirection": "ASC",
        "sortPriority": 1
        }
    ]
  }
}
```

The first event sent in the SSE response is the `METADATA` event, which contains the number of rows that will be returned. This event is followed by a stream of `DATA` events. Each `DATA` event contains some rows from the total set of rows to be returned. If an error is encountered while streaming, an `ERROR` event is sent, which contains the `TableauError` object.

Here’s an example of a server sent events response:

```
[
  {
    "event": "METADATA",
    "data": {
      "rowCount": 59
    }
  },
  {
    "event": "DATA",
    "data": [
      { "State/Province": "Alabama" },
      { "State/Province": "Alberta" },
      { "State/Province": "Arizona" },
      { "State/Province": "Arkansas" },
      { "State/Province": "British Columbia" }
    ]
  },
  {
    "event": "DATA",
    "data": [
      { "State/Province": "California" },
      { "State/Province": "Colorado" },
      { "State/Province": "Connecticut" },
      { "State/Province": "Delaware" },
      { "State/Province": "District of Columbia" }
    ]
  }
]
```

### SSE and error checking

Because the SSE results are streamed, errors are returned differently depending on when they occur:

- If an error occurs before any results are streamed, the response uses the normal REST pattern: a non-200 HTTP status code and a response body containing a `TableauError` object. For example, an invalid request is rejected, and the query is never run. Here’s an example of an SSE response when an error occurs in the query request (before streaming): `{ "errorCode": "400803", "message": "Unknown Field: Stae/Province.", "datetime": "2025-10-30T20:54:18.796332568Z" } `
- If an error occurs after streaming response has started, the HTTP status code and headers have already been sent and can’t be changed. In this case, you might still receive HTTP 200 even though the query request failed. When the error occurs during streaming, the data stops being added, an `error` property is added to the results, and the response is completed. The `error` property contains a `TableauError` object (which includes a VDS error code. The first three digits of the error code correspond to the HTTP status). Because an error can occur even when the HTTP status indicates success, check both the HTTP status code and whether an `error` property is present in the response body. The following example shows an SSE response when an error is encountered during streaming: `[ { "event": "METADATA", "data": { "rowCount": 1921 } }, { "event": "DATA", "data": [ { "Category": "Furniture", "Sub-Category": "Bookcases", "Country/Region": "Canada", "Product Name": "Atlantic Metals Mobile 2-Shelf Bookcases, Custom Colors", "AVG(Sales)": 72.294 }, { "Category": "Furniture", "Sub-Category": "Bookcases", "Country/Region": "Canada", "Product Name": "Bush Westfield Collection Bookcases, Fully Assembled", "AVG(Sales)": 168.31 } ] }, { "event": "ERROR", "data": { "errorCode": "500000", "datetime": "2025-10-30T22:33:15.3807749Z", "debug": null } } ] `

---

# Table Calculations Prerequisites

This section assumes a basic understanding of table calculations, including addressing and partitioning. For more information, see the following documentation topics:

- [Transform Values with Table Calculations](https://help.tableau.com/current/pro/desktop/en-us/calculations_tablecalculations.htm)
- [Quick Table Calculations](https://help.tableau.com/current/pro/desktop/en-us/calculations_tablecalculations_quick.htm)
- [Customize Table Calculations](https://help.tableau.com/current/pro/desktop/en-us/calculations_tablecalculations_custom.htm)

Quick table calculations are a nice interface that matches the *Quick Table Calculation* dialog in Tableau. However, anything you can do with quick table calculations, you can also do with custom table calculations. Custom table calculations are the equivalent of typing in a calculation.

---

# Table Calculations Overview

- Supported calculations
- Table calculation types
- Extra customizations per type
    - Rank table calculation options
    - Difference table calculation options
    - Moving calculation options
    - Custom sort calculation options

A table calculation is a transformation you apply to the values in a query. Table calculations are a special type of calculated field that computes on *local* data in Tableau. They are calculated based on what is currently in the query.

You can use table calculations for a variety of purposes, including:

- Transforming values to rankings
- Transforming values to show running totals
- Transforming values to show a percent of total

Here is an example of a visualization with no table calculations.

The equivalent VizQL Data Service (VDS) query with no table calculation applied is:

```
"query": {
   "fields": [
       {
           "fieldCaption": "Region"
       },
	 {
           "fieldCaption": "Order Date",
           "function": "YEAR"
       },{
           "fieldCaption": "Sales",
           "function": "SUM"
       },
       {
           "fieldCaption": "Profit",
           "function": "SUM"
       }
   ]
 }
```

For any VDS query or in the Tableau user interface, there is a virtual table that is determined by the dimensions in the query. Think of these dimensions as the ones that are “in the view” in the Tableau user interface. In the preceding example, these dimensions are `YEAR(Order Date)` and `Region`.

## Supported calculations

VDS supports the following calculations:

- Basic row level calculations
- Aggregate calculations
- Table calculations and predictive modeling calculations
- Level of detail (LOD) expressions
- Logical calculations (if / then / else)
- String / date / number / type conversions
- Parameter-based calculations
- User functions

Each table calculation requires you to specify a field to create the table calculation from, a `tableCalcType`, and the `dimensions` to include. The dimensions you include in your query and the ordering determine the **Compute Using**, that is, the addressing and partitioning fields.

All table calculations must have the field on which you are operating, the table calculation type, and the list of dimensions (see more below). For each table calculation type, there could be additional requirements or customizations in accordance with the Quick Table calculations dialog.

OpenAPI basics for a table calculation field:

```
{
  "fieldCaption": "string",
  "tableCalculation": {
    "tableCalcType": "string",
    "dimensions": [ // Same as "Specific Dimensions" listed above      {
        "fieldCaption": "string",
      }
    ]
  }
}
```

```
dimensions
```

## Table calculation types

The VDS OpenAPI schema allows for the following table calculation types. Depending on the type of table calculation, you will be required to provide extra fields as well, which corresponds to the various table calculation types that you would see in the Tableau UI dialog and what you would need to provide for each of those.

```
"tableCalcType": {
    "type": "string",
    "enum": [
        "CUSTOM",
        "DIFFERENCE_FROM",
        "PERCENT_DIFFERENCE_FROM",
        "PERCENT_FROM",
        "PERCENT_OF_TOTAL",
        "RANK",
        "PERCENTILE",
        "RUNNING_TOTAL",
        "MOVING_CALCULATION"
    ]
```

We will now walk through examples of each type of table calculation.

## Extra customizations per type

Different types of table calculations have advanced configuration options. You can see these in the Tableau user interface dialog as well if you’re creating Quick Table calculations.

### Rank table calculation options

```
{
  "rankType": "COMPETITION|MODIFIED COMPETITION|DENSE|UNIQUE",
  "direction": "ASC|DESC"
}
```

- COMPETITION: Standard ranking (1, 2, 3, 4…)
- MODIFIED COMPETITION: Modified ranking (1, 2, 3, 3, 5…)
- DENSE: Dense ranking (1, 2, 3, 3, 4…)
- UNIQUE: Unique ranking (1, 2, 3, 4, 5…)

### Difference table calculation options

```
{
  "relativeTo": "PREVIOUS|NEXT|FIRST|LAST",
  "levelAddress": {
    "fieldCaption": "string",
    "function": "string"
  }
}
```

### Moving calculation options

```
{
  "aggregation": "SUM|AVG|MIN|MAX",
  "previous": -2, // number of periods before current
  "next": 0, // number of periods after current
  "includeCurrent": true, // include current period
  "fillInNull": false // fill null values
}
```

### Custom sort calculation options

Additionally, many table calculation types allow for a custom sort. You can define another field in the query to sort on.

```
{
  "customSort": {
    "fieldCaption": "string",
    "function": "string",
    "direction": "ASC|DESC"
  }
}
```

---

# Table Calculation Query Examples

- Example 1: Rank profit by region and year
- Example 2: Percent of total profit by region and year
- Example 3: Running total profit by region and year
- Example 4: Difference from previous year’s profit
- Example 5: Moving average profit (three-year window)
- Secondary table calculations
    - Order of operations
    - Supported combinations
    - Use cases
    - Common patterns
    - Example: Running total with percent difference secondary calculation

## Example 1: Rank profit by region and year

This example ranks profit values within each region-year combination.

```
"query": {
    "fields": [
      {
        "fieldCaption": "Region"
      },
      {
        "fieldCaption": "Order Date",
        "function": "YEAR"
      },
      {
        "fieldCaption": "Sales",
        "function": "SUM"
      },
      {
        "fieldCaption": "Profit",
        "function": "SUM"
      },
      {
        "fieldCaption": "Profit",
        "function": "SUM",
        "tableCalculation": {
          "tableCalcType": "RANK",
          "dimensions": [
            {
              "fieldCaption": "Region"
            },
            {
              "fieldCaption": "Order Date",
              "function": "YEAR"
            }
          ],
           "rankType": "COMPETITION"
        }
      }
    ]
  }
```

In the response, each region-year combination will have profit values ranked from highest to lowest.

## Example 2: Percent of total profit by region and year

This example shows what percentage each profit value represents of the total profit for that region-year.

```
"query": {
  "fields": [
    {
      "fieldCaption": "Region"
    },
    {
      "fieldCaption": "Order Date",
      "function": "YEAR"
    },
    {
      "fieldCaption": "Sales",
      "function": "SUM"
    },
    {
      "fieldCaption": "Profit",
      "function": "SUM"
    },
    {
      "fieldCaption": "Profit",
      "function": "SUM",
      "tableCalculation": {
        "tableCalcType": "PERCENT_OF_TOTAL",
        "dimensions": [
          {
            "fieldCaption": "Region"
          },
          {
            "fieldCaption": "Order Date",
            "function": "YEAR"
          }
        ]
      }
    }
  ]
}
```

In the response, each profit value will be shown as a percentage of the total profit for that specific region and year.

## Example 3: Running total profit by region and year

This example shows cumulative profit over time for each region.

```
"query": {
    "fields": [
        {
            "fieldCaption": "Region"
        },
        {
            "fieldCaption": "Order Date",
            "function": "YEAR"
        },
        {
            "fieldCaption": "Sales",
            "function": "SUM"
        },
        {
            "fieldCaption": "Profit",
            "function": "SUM"
        },
        {
            "fieldCaption": "Profit",
            "function": "SUM",
            "tableCalculation": {
                "tableCalcType": "RUNNING_TOTAL",
                "dimensions": [
                    {
                        "fieldCaption": "Region"
                    },
                    {
                        "fieldCaption": "Order Date",
                        "function": "YEAR"
                    }
                ],
                "restartEvery": {
                    "fieldCaption": "Order Date",
                    "function": "YEAR"
                }
            }
        }
    ]
}
```

## Example 4: Difference from previous year’s profit

This example shows how much profit changed compared to the previous year.

```
"query": {
  "fields": [
    {
      "fieldCaption": "Region"
    },
    {
      "fieldCaption": "Order Date",
      "function": "YEAR"
    },
    {
      "fieldCaption": "Sales",
      "function": "SUM"
    },
    {
      "fieldCaption": "Profit",
      "function": "SUM"
    },
    {
      "fieldCaption": "Profit",
      "function": "SUM",
      "tableCalculation": {
        "tableCalcType": "DIFFERENCE_FROM",
        "dimensions": [
          {
            "fieldCaption": "Order Date",
            "function": "YEAR"
          }
        ],
        "relativeTo": "PREVIOUS"
      }
    }
  ]
}
```

The response shows the absolute difference in profit compared to the previous year for each region.

## Example 5: Moving average profit (three-year window)

This example shows a three-year moving average of profit for each region.

```
"query": {
  "fields": [
    {
      "fieldCaption": "Region"
    },
    {
      "fieldCaption": "Order Date",
      "function": "YEAR"
    },
    {
      "fieldCaption": "Sales",
      "function": "SUM"
    },
    {
      "fieldCaption": "Profit",
      "function": "SUM"
    },
    {
      "fieldCaption": "Profit",
      "function": "SUM",
      "tableCalculation": {
        "tableCalcType": "MOVING_CALCULATION",
        "dimensions": [
          {
            "fieldCaption": "Region"
          },
          {
            "fieldCaption": "Order Date",
            "function": "YEAR"
          }
        ],
        "aggregation": "SUM",
        "previous": -2,
        "next": 1,
        "includeCurrent": true
      }
    }
  ]
}
```

## Secondary table calculations

There is an option to add secondary table calculations.

### Order of operations

- Primary calculation is applied first
- Secondary calculation is applied to the results of the primary calculation

### Supported combinations

- RUNNING_TOTAL can have any secondary calculation
- MOVING_CALCULATION can have any secondary calculation
- Other table calculation types do not support secondary calculations

### Use cases

- **Smoothing**: Apply moving average to running totals for trend analysis
- **Ranking**: Rank the results of running totals or moving averages
- **Normalization**: Convert running totals to percentages or percentiles
- **Growth Analysis**: Show growth rates of cumulative values

### Common patterns

- **Running Total + Percent of Total:** Show cumulative contribution to total
- **Moving Average + Rank**: Rank smoothed values
- **Running Total + Difference**: Show growth of cumulative values
- **Moving Average + Percentile**: Show relative position of smoothed values

### Example: Running total with percent difference secondary calculation

This example shows how to first calculate running totals, then show the percentage change in running totals.

```
"query": {
  "fields": [
    {
      "fieldCaption": "Region"
    },
    {
      "fieldCaption": "Order Date",
      "function": "YEAR"
    },
    {
      "fieldCaption": "Sales",
      "function": "SUM"
    },
    {
      "fieldCaption": "Profit",
      "function": "SUM"
    },
    {
      "fieldCaption": "Profit",
       "function": "SUM",
      "tableCalculation": {
        "tableCalcType": "RUNNING_TOTAL",
        "dimensions": [
          {
            "fieldCaption": "Region"
          },
          {
            "fieldCaption": "Order Date",
            "function": "YEAR"
          }
        ],
        "aggregation": "SUM",
        "secondaryTableCalculation": {
          "tableCalcType": "PERCENT_DIFFERENCE_FROM",
          "dimensions": [
            {
              "fieldCaption": "Region"
            },
            {
              "fieldCaption": "Order Date",
              "function": "YEAR"
            }
          ],
          "relativeTo": "PREVIOUS"
        }
      }
    }
  ]
}
```

---

# Custom Table Calculations

- Common custom table calculation patterns
    - Difference from previous
    - Percent change
    - Year-over-year growth
    - Running total
    - Moving average

Custom table calculations allow you to write your own Tableau calculation formulas using the CUSTOM table calculation type. This provides the flexibility to create complex calculations that go beyond the standard quick table calculations available in Tableau Desktop. For more information, see the Tableau documentation for:

- [Customize Table Calculations](https://help.tableau.com/current/pro/desktop/en-us/calculations_tablecalculations_custom.htm)
- [Table Calculation Functions](https://help.tableau.com/current/pro/desktop/en-us/functions_functions_tablecalculation.htm)

Write a table calculation in the “calculation” field of the Table Calculation Field and give it a name.

For example:

```
"query": {
    "fields": [
        {
            "fieldCaption": "Region",
            "sortPriority": 1
        },{
              "fieldCaption": "Segment",
              "sortPriority": 2
        }, {
              "fieldCaption": "Order Date",
              "function": "YEAR",
              "sortPriority": 3
        }, {
            "fieldCaption": "MyDifferenceCalc",
            "calculation": "ZN(SUM([Sales])) - LOOKUP(ZN(SUM([Sales])), -1)",
            "tableCalculation": {
                "tableCalcType": "CUSTOM",
                "dimensions": [
                    {
                        "fieldCaption": "Region"
                    }, {
                        "fieldCaption": "Segment"
                    }, {
                        "fieldCaption": "Order Date",
                        "function": "YEAR"
                    }
                ]
            }
        }
    ]
  }
```

The preceding example does the following:

- `ZN(SUM([Sales]))`: Gets the current row’s sales value (ZN handles nulls).
- `LOOKUP(ZN(SUM([Sales])), \-1)`: Gets the previous row’s sales value.
- `-`: Subtracts the previous value from the current value.

The result shows the difference in sales from the previous period.

## Common custom table calculation patterns

### Difference from previous

This example calculates the absolute difference from the previous value, using the formula, `ZN(SUM(\[Sales\])) - LOOKUP(ZN(SUM(\[Sales\])), -1)`.

```
{
  "fieldCaption": "Sales Difference",
  "calculation": "ZN(SUM([Sales])) - LOOKUP(ZN(SUM([Sales])), -1)",
  "tableCalculation": {
    "tableCalcType": "CUSTOM",
    "dimensions": [
      {
        "fieldCaption": "Region"
      },
      {
        "fieldCaption": "Order Date",
        "function": "YEAR"
      }
    ]
  }
}
```

### Percent change

This example calculates the percentage change from the previous value, using the formula: `(ZN(SUM([Sales])) - LOOKUP(ZN(SUM([Sales])), -1)) / LOOKUP(ZN(SUM([Sales])), -1)`.

```
{
  "fieldCaption": "Sales % Change",
  "calculation": "(ZN(SUM([Sales])) - LOOKUP(ZN(SUM([Sales])), -1)) / LOOKUP(ZN(SUM([Sales])), -1)",
  "tableCalculation": {
    "tableCalcType": "CUSTOM",
    "dimensions": [
      {
        "fieldCaption": "Region"
      },
      {
        "fieldCaption": "Order Date",
        "function": "YEAR"
      }
    ]
  }
}
```

### Year-over-year growth

This example calculates year-over-year growth, using the formula, `SUM([Sales]) - LOOKUP(SUM([Sales]), -4)) / LOOKUP(SUM([Sales]), -4)`.

```
{
  "fieldCaption": "YoY Growth",
  "calculation": "(SUM([Sales]) - LOOKUP(SUM([Sales]), -4)) / LOOKUP(SUM([Sales]), -4)",
  "tableCalculation": {
    "tableCalcType": "CUSTOM",
    "dimensions": [
      {
        "fieldCaption": "Region"
      },
      {
        "fieldCaption": "Order Date",
        "function": "YEAR"
      }
    ]
  }
}
```

### Running total

This example calculates cumulative totals, using the formulat, `RUNNING_SUM(SUM([Sales]))`.

```
{
  "fieldCaption": "Running Total Sales",
  "calculation": "RUNNING_SUM(SUM([Sales]))",
  "tableCalculation": {
    "tableCalcType": "CUSTOM",
    "dimensions": [
      {
        "fieldCaption": "Region"
      },
      {
        "fieldCaption": "Order Date",
        "function": "YEAR"
      }
    ]
  }
}
```

### Moving average

This example calculates a three-period moving average, using the formula, `WINDOW\_AVG(SUM(\[Sales\]), \-2, 0\)`.

```
{
  "fieldCaption": "Moving Average Sales",
  "calculation": "WINDOW_AVG(SUM([Sales]), -2, 0)",
  "tableCalculation": {
    "tableCalcType": "CUSTOM",
    "dimensions": [
      {
        "fieldCaption": "Region"
      },
      {
        "fieldCaption": "Order Date",
        "function": "YEAR"
      }
    ]
  }
}
```

---

# Nested Table Calculations

VDS supports [nested table calculations](https://help.tableau.com/current/pro/desktop/en-us/calculations_tablecalculations_custom.htm#nested-table-calculations). If you have one table calculation within another one, you can set the dimensions for compute Using independently for each table calculation referenced. To configure nested calculations independently, use:

```
tableCalcType": "NESTED"
```

For example, if you have three calculations saved on your published data source:

- **1-nest,** with the formula `TOTAL(SUM([Sales]))` (a table calc)
- **2-nest**, with the formula `TOTAL(SUM([Profit]))`(also a table calc)
- **3-nest**, with the formula `[1-nest] + [2-nest]` (not a table calc in itself, but has two nested table calculations)

Our goal is to query the calculation **1-nest**. In this case, you want to compute the dimensions of 1-nest and 2-nest independently. You could do this with the following query:

```
"query": {
  "fields": [
    {
      "fieldCaption": "Region",
      "sortPriority": 1
    },
    {
      "fieldCaption": "Segment",
      "sortPriority": 2
    },
    {
      "fieldCaption": "Order Date",
      "function": "YEAR",
      "sortPriority": 3
    },
    {
      "fieldCaption": "3-nest",
      "tableCalculation": {
        "tableCalcType": "CUSTOM",
        "dimensions": [
        ]
      },
      "nestedTableCalculations": [
        {
          "fieldCaption": "1-nest",
          "tableCalcType": "NESTED",
          "dimensions": [
            {
              "fieldCaption": "Region"
            },
            {
              "fieldCaption": "Segment"
            },
            {
              "fieldCaption": "Order Date",
              "function": "YEAR"
            }
          ]
        },
        {
          "fieldCaption": "2-nest",
          "tableCalcType": "NESTED",
          "dimensions": [
            {
              "fieldCaption": "Region"
            },
            {
              "fieldCaption": "Segment"
            }
          ],
          "restartEvery": {
            "fieldCaption": "Region"
          }
        }
      ]
    }
  ]
}
```

Because **1-nest** isn’t a table calculation in itself, the dimensions list is empty.

Let’s say you want to query another calculation saved on your published data source, called **4-nest**. You intend to use the formula `WINDOW_SUM(SUM([SALES]), -2, 0) - [2-nest]`.

Because **4-nest** is a table calculation in itself (`WINDOW_SUM`), but so is **2-nest,**. You can add dimensions for **Compute Using** for each calculation.

```
"query": {
   "fields": [
     {
       "fieldCaption": "Region",
       "sortPriority": 1
     },
     {
       "fieldCaption": "Segment",
       "sortPriority": 2
     },
     {
       "fieldCaption": "Order Date",
       "function": "YEAR",
       "sortPriority": 3
     },
     {
       "fieldCaption": "4-nest",
       "tableCalculation": {
         "tableCalcType": "CUSTOM",
         "dimensions": [
           {
             "fieldCaption": "Region"
           },
           {
             "fieldCaption": "Segment"
           },
           {
             "fieldCaption": "Order Date",
             "function": "YEAR"
           }
         ]
       },
       "nestedTableCalculations": [
         {
           "fieldCaption": "2-nest",
           "tableCalcType": "NESTED",
           "dimensions": [
             {
               "fieldCaption": "Region"
             },
             {
               "fieldCaption": "Segment"
             }
           ]
         }
       ]
     }
   ]
 }
```

---

# VizQL Data Service ( 1.0 )

An API to query Tableau published data sources

▷ Download the OpenAPI spec for these operations..

# VizQL Data Service Guide

▷ View the VizQL Data Service Guide.

## Request data source metadata

Requests metadata for a specific data source. The metadata provides information about the data fields, such as field names, data types, and descriptions.

##### Authorizations:

##### Request Body schema: application/json

| datasourcerequired | object ( Datasource ) |
|---|---|
| options | object ( QueryOptions ) Some optional metadata that can be used to adjust the behavior of an endpoint. |

### Responses

200 response

Unexpected error

### Request samples

- Payload

```
{"datasource": {"datasourceLuid": "string","workbookDatasourceId": "string","connections": [{"connectionLuid": "string","connectionUsername": "string","connectionPassword": "string"}]},"options": {"debug": false,"bypassMetadataCache": false,"withNewSession": false,"interpretFieldCaptionsAsFieldNames": false,"includeHiddenFields": false,"includeGroupFormulas": false}}
```

### Response samples

- 200
- default

```
{"data": [{"fieldName": "string","fieldCaption": "string","dataType": "UNSPECIFIED","fieldRole": "UNSPECIFIED","fieldType": "UNSPECIFIED","defaultAggregation": "UNSPECIFIED","columnClass": "UNSPECIFIED","formula": "string","groupFormula": {"baseFieldName": "string","groupings": [{"alias": "string","members": [null]}],"hasIncludeOther": false},"logicalTableId": "string","description": "string","imageRole": "UNSPECIFIED","hidden": true,"defaultFormatting": {"decimalPlaces": "string"},"isLODCalc": true,"aliases": [{"member": null,"value": null}]}],"extraData": {"parameters": [{"parameterType": "UNSPECIFIED","parameterName": "string","parameterCaption": "string","dataType": "UNSPECIFIED","value": null}]}}
```

## Query data source

Queries a specific data source and returns the resulting data.

##### Authorizations:

##### Request Body schema: application/json

| datasourcerequired | object ( Datasource ) |
|---|---|
| queryrequired | object ( Query ) The query is the fundamental interface to the VizQL Data Service. It holds the specific semantics to perform against the data source. A query consists of an array of fields to query against, and an optional array of filters to apply to the query. |
| options | object ( QueryDatasourceOptions ) Some optional metadata that can be used to adjust the behavior of an endpoint. |

### Responses

200 response

Unexpected error

### Request samples

- Payload

```
{"datasource": {"datasourceLuid": "string","workbookDatasourceId": "string","connections": [{"connectionLuid": "string","connectionUsername": "string","connectionPassword": "string"}]},"query": {"fields": [{"fieldCaption": "string","fieldAlias": "string","maxDecimalPlaces": 0,"sortDirection": "ASC","sortPriority": 1}],"filters": [{"field": {"fieldCaption": "string"},"filterType": "QUANTITATIVE_DATE","context": false}],"parameters": [{"parameterCaption": "string","value": null}]},"options": {"debug": false,"bypassMetadataCache": false,"withNewSession": false,"interpretFieldCaptionsAsFieldNames": false,"includeHiddenFields": false,"includeGroupFormulas": false,"disaggregate": false,"returnFormat": "OBJECTS","rowLimit": 1,"returnServerSentEvents": false}}
```

### Response samples

- 200
- default

```
{"data": [null],"error": { }}
```

## Send a simple request

Sends a request that can be used for testing or doing a health check.

### Responses

200 response

### Response samples

- 200

```
"string"
```

## Request data source model

Requests the data model for a specific data source. The data model provides information about the structure and relationships in the data source.

##### Authorizations:

##### Request Body schema: application/json

| datasourcerequired | object ( Datasource ) |
|---|---|
| options | object ( QueryOptions ) Some optional metadata that can be used to adjust the behavior of an endpoint. |

### Responses

200 response

Unexpected error

### Request samples

- Payload

```
{"datasource": {"datasourceLuid": "string","workbookDatasourceId": "string","connections": [{"connectionLuid": "string","connectionUsername": "string","connectionPassword": "string"}]},"options": {"debug": false,"bypassMetadataCache": false,"withNewSession": false,"interpretFieldCaptionsAsFieldNames": false,"includeHiddenFields": false,"includeGroupFormulas": false}}
```

### Response samples

- 200
- default

```
{"logicalTables": [{"logicalTableId": "string","caption": "string","description": "string"}],"logicalTableRelationships": [{"fromLogicalTable": {"logicalTableId": "string"},"toLogicalTable": {"logicalTableId": "string"},"expression": {"op": "string","relationships": [{"operator": "=","fromField": "string","toField": "string"}]}}]}
```

## List supported Tableau functions for a datasource

Returns the list of Tableau function names supported for the specified datasource. The 200 response is an array of strings. Example response: ["SUM", "AVG", "MIN", "MAX"].

##### Authorizations:

##### Request Body schema: application/jsonrequired

| datasourcerequired | object ( Datasource ) |
|---|---|
| options | object ( QueryOptions ) Some optional metadata that can be used to adjust the behavior of an endpoint. |

### Responses

List of supported function object for the datasource

Unexpected error

### Request samples

- Payload

```
{"datasource": {"datasourceLuid": "string","workbookDatasourceId": "string","connections": [{"connectionLuid": "string","connectionUsername": "string","connectionPassword": "string"}]},"options": {"debug": false,"bypassMetadataCache": false,"withNewSession": false,"interpretFieldCaptionsAsFieldNames": false,"includeHiddenFields": false,"includeGroupFormulas": false}}
```

### Response samples

- 200
- default

```
[{"name": "string","overloads": [{"arg_types": ["UNSPECIFIED"],"return_type": "UNSPECIFIED"}]}]
```

---

# VizQL OpenAPI Schema

You can access the VizQL Data Service OpenAPI schema in the [VizQL Data Service GitHub repository](https://github.com/tableau/VizQL-Data-Service/blob/main/VizQLDataServiceOpenAPISchema.json)

| Tableau Version and Availability | VizQL Data Service OpenAPI schema |
|---|---|
| Tableau Cloud (July 2026) , Tableau Server 2026.3 | [VizQLDataServiceOpenAPISchema.json](https://github.com/tableau/VizQL-Data-Service/blob/release-20262.0/VizQLDataServiceOpenAPISchema.json) |
| Tableau Cloud (February 2026) | [VizQLDataServiceOpenAPISchema.json](https://github.com/tableau/VizQL-Data-Service/blob/release-20261.0/VizQLDataServiceOpenAPISchema.json) |
| Tableau Cloud (October 2025), Tableau Server 2025.3 | [VizQLDataServiceOpenAPISchema.json](https://github.com/tableau/VizQL-Data-Service/blob/release-20253.0/VizQLDataServiceOpenAPISchema.json) |
| Tableau Cloud (June 2025) | [VizQLDataServiceOpenAPISchema.json](https://github.com/tableau/VizQL-Data-Service/blob/release-20252.0/VizQLDataServiceOpenAPISchema.json) |
| Tableau Cloud (February 2025), Tableau Server 2025.1 | [VizQLDataServiceOpenAPISchema.json](https://github.com/tableau/VizQL-Data-Service/blob/release-20251.0/VizQLDataServiceOpenAPISchema.json) |

---

# Error Codes

| HTTP Status | Error Code | Condition | Details |
|---|---|---|---|
| 400 | 400000 | Bad request | The content of the request body is invalid. Check for missing or incomplete JSON. |
| 400 | 400800 | Invalid formula for calculation | The incoming request has an invalid formula in the calculation. |
| 400 | 400802 | Invalid API request | The incoming request isn’t valid per the OpenAPI specification. |
| 400 | 400803 | Validation failed | The incoming request isn’t valid per the validation rules. [Calculations in Tableau](https://help.tableau.com/current/pro/desktop/en-us/functions_operators.htm). |
| 400 | 400804 | Invalid calculation aggregation | The request has an invalid calculation aggregation. |
| 400 | 400805 | Results oversize | The query results are too large. |
| 401 | 401001 | Log-in error | The log-in failed for the given user. |
| 401 | 401002 | Invalid authorization credentials | The provided auth token is formatted incorrectly. |
| 403 | 403157 | Feature disabled | The feature is disabled. |
| 403 | 403800 | API access permission denied | The user doesn’t have API Access granted on the given data source. Set the **API Access** capability for the given data source to **Allowed**. For help, see [Permission Capabilities and Templates](https://help.tableau.com/current/online/en-us/permissions_capabilities.htm). |
| 404 | 404934 | Unknown field | The requested field doesn’t exist. |
| 404 | 404935 | Duplicate field caption | There are two or more fields with the same caption in the published data source. |
| 404 | 404950 | API endpoint not found | The request endpoint doesn’t exist. |
| 408 | 408000 | Request timeout | The request timed out. |
| 408 | 408801 | Request timeout | The request timed out. |
| 409 | 409000 | User already on site | HTTP status conflict. |
| 429 | 429000 | Too many requests | Too many requests in the allotted amount of time. For help, see Licensing and data transfer. |
| 499 | 499000 | Client disconnect | The client disconnected from the service. |
| 500 | 500000 | Internal server error | The request could not be completed. |
| 500 | 500810 | VizQL Data Service empty table response | The underlying data engine returned empty data value response. |
| 500 | 500811 | VizQL Data Service missing table | The underlying data engine returned empty metadata associated with response. |
| 500 | 500812 | Error while processing an error | Internal processing error. |
| 500 | 500813 | Parameter name undefined | There’s an invalid parameter in the data source and VDS can’t find what type parameter it is. |
| 500 | 500814 | Bin name undefined | There’s an invalid bin in the data source and VDS can’t find what the bin type is. |
| 500 | 500815 | Data source model undefined | VDS can’t find information about the data model. |
| 501 | 501000 | Not implemented | Can’t find response from upstream server. |
| 503 | 503800 | VizQL Data Service unavailable | The underlying data engine is unavailable. |
| 503 | 503801 | VizQL Data Service discovery error | The upstream service can’t be found. |
| 503 | 503802 | Tableau service unavailable | The underlying data engine is unavailable. |
| 503 | 503803 | Tableau service discovery error | The upstream service can’t be found. |
| 503 | 503804 | Tableau service session not found | The service session can’t be found. |
| 503 | 503805 | VizQL Data Service unavailable | The underlying data engine is unavailable. |
| 504 | 504000 | Tableau service timeout | The upstream service response timed out. |
| 504 | 504001 | Tableau service timeout | The upstream service response timed out. |

For more information, see [Handling Errors](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_concepts_errors.htm).

---

# Limitations

- Calculations
- Data sources
- Fields
- Filters
- Licensing and data transfer
- Queries
- Response size
- Other

## Calculations

VizQL Data Service (VDS) doesn’t support these calculation types:

- Spatial calculations
- Python or R calculations (like SCRIPT_REAL)
- Tableau Analytics Extensions calculations
- Pass-through calculations (like RAWSQL)
- Fiscal date calculations
- COUNT(table)
- Calculations on features we don’t support:
    - Sets
    - Combined fields

Also, you can’t reference a group in a calculation.

## Data sources

VDS doesn’t support [cube data sources](https://help.tableau.com/current/pro/desktop/en-us/cubes.htm#what-are-cube-data-sources).

## Fields

VDS doesn’t support querying for sets and combined fields. These field types aren’t returned as part of the Request data source metadata method.

## Filters

- VDS doesn’t support bins or groups in filters.
- Set filters, match filters, and relative date filters can’t have functions or calculations.
- The `field` property of a `filter` can contain a `fieldName`, or a `fieldName` and a `function`, for example Sales, or SUM of Sales. Within a query, you can only have one `filter` per `field`. So if you have a `filter` on SUM of Sales, you can’t add another `filter` in the same query for SUM of Sales. However, you can add a `filter` for Sales because it’s a different `field`.

## Licensing and data transfer

VDS is available for all license models. There’s a cap on usage determined by the number of [Tableau Creator licenses](https://www.tableau.com/pricing/tableau-license-types) assigned to a site. Each Creator license on a site raises the cap for the entire site by 100 queries per hour.

## Queries

VDS doesn’t support all features of Tableau services. For a full list of supported features, see Create a Query.

## Response size

In versions of Tableau before Tableau 2026.1, VDS has a response size limit of 1-GB. Any response size larger than that results in an error. To avoid such an error, we recommend that you apply a filter to the data to limit the response size.

Starting in Tableau 2026.1, there’s no longer a 1-GB limit on the response size of a query. In addition, the Query data source method has a 30-minute timeout. For more information, see Results handling and error checking.

## Other

VDS doesn’t support date-time aggregations (for example, `HOUR`, `MINUTE`, and so on).

---

# Troubleshooting

- Make sure your URL is correct by hitting the `simple-request` endpoint. If you see “ahoy” returned, you know you can reach the service.
- Turn on the `debug` flag in `options` to get a more detailed error message. ` { "datasource": { "datasourceLuid": "" }, "options": { "debug": true } } `
- For Tableau Server, examine the log files. For help, see [Tableau Server Logs and Log File Locations](https://help.tableau.com/current/server/en-us/logs_loc.htm).
- To debug log in issues, see [Testing and Troubleshooting REST API Calls](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_concepts_testing.htm).
- A common error is the following: ` {"errorCode":"400000", "message":"Unable to parse the request body. Please ensure it is formatted as valid JSON. (Illegal unquoted character ((CTRL-CHAR, code 10)): has to be escaped using backslash to be included in string value)."} ` This likely means that you have an extra indentation after your `datasource-luid`. Make sure the data source LUID is a single line.

---

# Using VizQL Data Service with Postman

[Postman](https://www.postman.com/) is a popular collaboration platform for application programming interface (API) development. It provides tools for designing, testing, and managing APIs, making it easier for you to work with and understand the functionality of APIs.

To help you onboard to VizQL Data Service, we’ve added a VizQL Data Service folder to the existing [Tableau API’s Postman collection](https://github.com/tableau/tableau-postman). (See here in [Postman](https://www.postman.com/salesforce-developers/salesforce-developers/collection/x06mp2m/tableau-apis)) For more information about signing in using the REST API, see [Get Started Tutorial Part 1: Tools, REST Basics, and Sign In](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_get_started_tutorial_part_1.htm).

Once you’ve authenticated and populated the `datasource-id` field in your environment variables, go to the VizQL Data Service folder to see query examples. You can also navigate to the Sample Workflows folder and find the VizQL Data Service example workflow.
