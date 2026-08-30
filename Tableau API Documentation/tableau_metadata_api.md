# Tableau Metadata API - Complete Documentation

Scraped from the official Tableau Metadata API documentation (https://help.tableau.com/current/api/metadata_api/en-us/).

## Contents

- [Documentation](#documentation)
- [GraphQL schema reference catalog](./tableau_metadata_api_reference.md)

---

# Documentation

# Introduction to Tableau Metadata API

The Tableau Metadata API discovers and indexes all of the content on your Tableau Cloud site or Tableau Server, including workbooks, data sources, flows, and metrics. Indexing is used to gather information about Tableau content, or metadata, about the schema and lineage of the content. Then from the metadata, Metadata API identifies all of the databases, files, and tables used by the content on your Tableau Cloud site or Tableau Server.

You can do the following tasks using the Metadata API:

- **Discover data** that’s associated with the content published to your Tableau Cloud site or your Tableau Server. Search for external assets like tables, databases, and data sources.
- **Track lineage** or the relationships between content and external assets, like data sources and workbooks. For example, identify which workbooks use a specific published data source.
- **Perform impact analysis**. Using upstream and downstream lineage information, you can evaluate impact of changes to content. For example, find all worksheets that depend on a database table column or identify the authors you should notify when a data source change occurs.

**In this section**

- What is Tableau metadata?
- Metadata API and GraphQL
- When to use the Metadata API?
- When to use the Tableau REST API?
- Differences between using GraphQL and REST

## What is Tableau metadata?

The Metadata API discovers, tracks, stores, and then surfaces information about Tableau content.

The content can be categorized by type (e.g., table or workbook). The content can be unique to Tableau (e.g., embedded data sources and calculated fields) and its external assets not unique to Tableau (e.g., database tables and columns). Both content and external assets can have information attached to them (e.g., tags and ratings). Both content and external assets can also have relationships to other content and external assets.

The relationships among the content and external assets and the information about each is the metadata.

## Metadata API and GraphQL

The Metadata API uses GraphQL, a query language for APIs that describes how to ask for and return only the data that you’re interested in.

For general information about GraphQL and what you can do with it, see [GraphQL.org [↗]](https://graphql.org).

## When to use the Metadata API?

The Metadata API is fast and flexible. Use the Metadata API when you are looking to find out specific information about the relationships between content and assets or their structures.

## When to use the Tableau REST API?

With regard to metadata, you can use the Tableau REST API to make similar queries as with the Metadata API. However, if your Tableau Cloud site or Tableau Server is licensed with Data Management, you can use the REST API to perform various write operations, such as add, update, or remove external assets; add, update, or remove permissions on external assets; and add additional metadata to the external assets like descriptions, certifications, and data quality warnings. For more information, see [Metadata Methods [↗]](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_metadata.htm) topic in the Tableau REST API.

**Note:** You can always use the Tableau REST API for additional tasks like publishing workbooks or data sources, or for administrative tasks, such as creating groups and adding or removing users.

## Differences between using GraphQL and REST

If you’re familiar with REST APIs then you understand the concepts of endpoints (or resources) and HTTP requests. For example, if you are using the Tableau REST API and you want to find all the workbooks on your Tableau Cloud site or Tableau Server, you can make a GET request to return that information. If you want to find out something else, you need to make another request to a different endpoint.

Like REST APIs, GraphQL is also served over HTTP. However, instead of sending requests to multiple endpoints to return the data of interest, you can make one query to one endpoint and have that filtered to return only what you ask for. A GraphQL server is associated with one endpoint.

GraphQL is based on the concept of a graph (queries) and the relationship between the types (objects) defined by the GraphQL schema.

---

# Get Started

To get started, review the requirements necessary to use the Tableau Metadata API.

**In this section**

- Prerequisites
- Enable the Tableau Metadata API for Tableau Server
- About querying the Metadata API
    - Who can query the Metadata API
- Query the API using the GraphQL endpoint
    - The endpoint URI
    - Query
    - Query example
    - Submitting the query
    - Query response
    - Permissions
    - Errors
- Explore the Metadata API schema using GraphiQL
    - About the GraphiQL query tool

## Prerequisites

To use the Tableau Metadata API, the following requirements must be met:

**For Tableau Cloud**

**Authentication token:** An authentication token is required to programmatically access the Metadata API through the GraphQL endpoint. The Metadata API uses the same authentication process and token as the Tableau REST API. For more information, see How to Authenticate.

**Note:** Metadata API is always enabled for Tableau Cloud.

**For Tableau Server**

- **Tableau Server 2019.3 or later**.
- **Tableau REST API must not be disabled.**
- **The Metadata API must be enabled**. For more information about enabling the Metadata API, see the section below.
- **Authentication token** to programmatically access the Metadata API through the GraphQL endpoint. The Metadata API uses the same authentication process and token as the Tableau REST API. For more information, see How to Authenticate.

## Enable the Tableau Metadata API for Tableau Server

The Metadata API is installed with Tableau Server but disabled by default.

A server admin must enable the Metadata API on Tableau Server using the `tsm maintenance metadata-services enable` command through the Tableau Services Manager (TSM) command line interface (CLI). For more information, see [tsm maintenance [↗]](https://help.tableau.com/current/server/en-us/cli_maintenance_tsm.htm#cat_enable) topic in the Tableau Server Help.

Running the command begins initial ingestion, which is a component of the indexing process for the Metadata API.

1. Open a command prompt as an admin on the initial node (where TSM is installed) in the cluster.
2. Run the command: `tsm maintenance metadata-services enable`

**Notes:** When running this command, keep the following points in mind:

- This command stops and starts some services used by Tableau Server, which causes certain functionality, such as the Recommendations capability, to be temporarily unavailable.
- A new index of metadata is created at this time. Running this command any subsequent times will create and replace the previous index.

After the requirements have been met and the Metadata API has been enabled (for Tableau Server only) you can learn about the GraphQL schema or start querying the Metadata API.

## About querying the Metadata API

To get the information you need from the Metadata API, you need to be able to write queries. Queries fetch metadata about the content published to your Tableau Cloud site or Tableau Server.

Before you can begin writing these queries, you need to understand what kind of Tableau Cloud or Tableau Server metadata is available through the Metadata API and the ways for accessing the metadata. For more information, see Understand the Metadata Model.

### Who can query the Metadata API

In general, all authorized users can query the Metadata API. For Tableau Server specifically, only after the Metadata API has been enabled, any authorized user can query the Metadata API.

However, what you can query and see with the Metadata API depends on whether your Tableau Cloud site or Tableau Server is licensed with Data Management.

|  | With Data Management | Without Data Management |
|---|---|---|
| **See metadata** | You can see your Tableau content and related external assets. You can also see external assets you’ve been granted explicit permissions to see. | You can see Tableau content. If “derived permissions” is enabled, you can also see related external assets. For more information about derived permissions: - For Tableau Cloud: [Permissions on external assets using derived permissions [↗]](https://help.tableau.com/current/online/en-us/dm_perms_assets.htm#derived) - For Tableau Server: [Permissions on external assets using derived permissions [↗]](https://help.tableau.com/current/server/en-us/dm_perms_assets.htm#derived) |
| **Edit metadata and manage permissions** | You can edit metadata and manage permissions for external assets that you’ve been granted explicit permissions to edit or manage permissions directly from your Tableau Cloud site or Tableau Server, or using the [metadata methods [↗]](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_metadata.htm) in the REST API. | Not supported. |

## Query the API using the GraphQL endpoint

Unlike a REST API, the Metadata API has only a single endpoint. To use the Metadata API, issue queries against this metadata api endpoint using GraphQL.

### The endpoint URI

`http://<server-name>/api/metadata/graphql`

**Note:**

- The API endpoint and the in-browser tool that you can use to query GraphQL have distinct but very similar URLs:
    - `https://<server-name>/api/metadata/graphql` is the API endpoint.
    - `https://<server-name>/metadata/graphiql/` is in-browser tool that you can use to query GraphQL.
- Issue API calls against the endpoint URI. If you’re looking for a way to query the metadata API in a browser, see Explore the Metadata API schema using GraphiQL.
- To use the GraphQL endpoint, you must authenticate using the Tableau REST API. For more information, see How to Authenticate topic.

### Query

To form a query using GraphQL, you must specify the objects, and in some cases, objects within those objects until a unit of data can be returned.

```
query <query-name>{
  <object> (<arguments>){
    <attribute>
    <attribute>{
      <attribute>
    }
  }
}
```

### Query example

```
query useMetadataApiToQueryOrdersDatabases{
  databases (filter: {name: "adventureworks"}){
    name
    tables{
      name
    }
  }
}
```

For more query examples, see the Example Queries topic.

### Submitting the query

#### POST and Content-Type application/json

You typically submit GraphQL queries using `POST` and an HTTP `Content-Type` header of `application/json`, with the JSON-encoded query in the HTTP request body. A typical HTTP request body with a JSON-encoded simple GraphQL query might look like:

```
{"query":"query myquery{databases {name id}}"}
```

#### POST and Content-Type application/graphql

You can also submit GraphQL queries using `POST` and an HTTP `Content-Type` header of `application/graphql`. A typical HTTP request body with the same simple GraphQL query as above would be:

```
query myquery{databases {name id}}
```

#### GET

You can also submit GraphQL queries using `GET` and a querystring, with no request body at all. The querystring is a field-value pair where the field is `query` and the value is the GraphQL query, URL encoded. A typical HTTP request using the same simple GraphQL query as above would result in a URI and querystring like:

```
https://tableau.example.com/api/metadata/graphql?query=query%20myquery%7Bdatabases%7Bname%20id%7D%7D'
```

#### More information

For more information on typical GraphQL requests and expected interactions, see [HTTP Methods, Headers, and Body [↗]](https://graphql.org/learn/serving-over-http/#http-methods-headers-and-body) on the GraphQL.org website.

### Query response

The query returns only the data you specify in same shape as your query.

For the query example used above, you will see the following query response.

```
{
  "data": {
    "databases": [
      {
        "name": "adventureworks",
        "tables": [
          {
            "name": "AWBuildVersion"
          }
        ]
      },
      {
        "name": "adventureworks",
        "tables": [
          {
            "name": "SalesTerritory"
          },
          {
            "name": "Department"
          },
          {
            "name": "Culture"
          },
          {
            "name": "Address"
          },
          {
            "name": "Product"
          }
        ]
      },
      {
        "name": "adventureworks",
        "tables": [
          {
            "name": "CustomerAddress"
          },
          {
            "name": "Address"
          },
          {
            "name": "Customer"
          }
        ]
      }
    ]
  }
}
```

### Permissions

Your query results are scoped to the permissions you’ve been granted. For more information, see How Permissions Work topic.

### Errors

See Common Errors topic.

## Explore the Metadata API schema using GraphiQL

One way to quickly get started with GraphQL queries and explore the Metadata API is to test queries against the schema. You can explore the schema using GraphiQL, which is an interactive in-browser tool. You can change the query as you like and see the results immediately.

**Note:** To access GraphiQL, you must sign in (authenticate) to your Tableau Cloud site or Tableau Server. The data you can see, specifically about external assets, is scoped to the permissions you’ve been granted.

1. Open a browser and sign in to [Tableau Cloud [↗]](https://online.tableau.com) or Tableau Server 2019.3 (or later).
2. Copy the following partial URL: `/metadata/graphiql/`
3. In the browser’s address bar, delete everything after the “`.com`”.
4. Paste the partial URL after the “`.com`” and press ENTER or RETURN.

For example, if your site’s name is “MYCO” and the site URL is:

`https://us-west-2b.online.tableau.com/#/site/MYCO/explore`

The GraphiQL tool URL is:

`https://us-west-2b.online.tableau.com/metadata/graphiql/`

**Note:**

- The API endpoint and the in-browser tool that you can use to query GraphQL have distinct but very similar URLs:
    - `https://<server-name>/api/metadata/graphql` is the API endpoint.
    - `https://<server-name>/metadata/graphiql/` is in-browser tool that you can use to query GraphQL.
- You can also access the GraphiQL in-browser tool from Tableau Catalog in Tableau Cloud site or Tableau Server by clicking **Query metadata (GraphiQL)** in the upper-right corner of the **External Assets** page.

### About the GraphiQL query tool

The GraphiQL query tool is comprised of several parts.

1. **History pane:** After you have written and run a query, it’s saved to a list so that you can reuse the query at another time. To toggle the History pane, click the **History** button in the toolbar.
2. **Left pane:** Build queries in the left pane. To run a query, click the play button (Execute Query) in the Toolbar. For more information about writing queries, see Example Queries.
3. **Query Variables pane:** Use this pane to define the variables that you pass to your queries. and mutations
4. **Toolbar:** Use the toolbar to run queries, see a history of queries that you’ve run, and to explore the GraphQL schema.
5. **Middle pane:** See the results of and validate your queries in the right pane.
6. **Right pane:** Toggle this pane by clicking **Documentation Explorer** to show the GraphQL schema. You can search the schema to see the objects that are implemented by the Metadata API and the metadata that you can query.

---

# Core Concepts and Definitions

This page describes the core concepts and definitions that you’ll see throughout this documentation. These terms and concepts can help you understand the components of the Metadata API, using GraphQL, and how these components relate to what you see and interact with on Tableau Cloud or Tableau Server.

**In this section**

- General terms
- GraphQL-specific terms

## General terms

- **Catalog** - a tool that ingests, indexes, and centralizes metadata about your Tableau Cloud or Tableau Server content and their assets. A catalog enables you to build queries to access the metadata.
- **Content** - a set of objects (and their metadata) that you can interact with using the Metadata API. Content is typically Tableau-specific objects that you can publish to or create on Tableau Cloud or Tableau Server. Content can include workbooks and projects.
- **Assets** - a set of objects (and their attributes) that you can interact with using the Metadata API. Assets are external to Tableau and therefore non-Tableau specific objects. Assets include databases and tables.
- **Metadata** - the information about your content and assets that you can query using the Metadata API.
- **Lineage** - reveals the origin of an object and its relationship to other related objects in Tableau Cloud or Tableau Server.
- **Impact analysis** - using lineage, understand impact of a change through upstream and downstream relationships among objects.
- **Metadata model** - the underling rules that Tableau uses to define the roles, relationships, and attributes of a set of related objects.
- **GraphQL schema** - describes the functionality available through the Metadata API.
- **Query** - a request for data.
- **Object** - the item you interact with on Tableau Cloud or Tableau Server.
- **Attribute** - a component or characteristic of an object.
- **Upstream (object)** - when traversing lineage, whatever information that is above the object you’re evaluating.
- **Downstream (object)** - when traversing lineage, whatever information that is below the objects you’re evaluating.
- **Shortcut** - a GraphQL field enabled through the Metadata API that queries for common upstream and downstream objects.
- **Field description inheritance** - describes field description attributes that can be inherited from upstream objects.
- **Linked flow** - a shortcut type that traverses lineage of flows and provides structural metadata about relationships of upstream or downstream flows.

## GraphQL-specific terms

- **Graph** - a GraphQL term for a snapshot of the relationship between objects. Graphs are created to get a real-time, real-world shape of your Tableau Cloud or Tableau Server objects. In this documentation, we will refer to a graph as a “query.”
- **Root object** - a GraphQL term for the most fundamental type of operation that the GraphQL schema supports. The Metadata API supports a “read” query type.
- **Query** - a GraphQL term for a “read” type of a root object.
- **Mutation** - a GraphQL term for a “write” type of root object. To add or update metadata, use the Tableau Server REST API. - **Input object** -
- **Node** - a GraphQL term for an implementation of the catalog. A node is a representation of data.
- **Edge** - a GraphQL term for the connection between nodes. An edge represents some type of relationship between two nodes.
- **Interface** - A GraphQL term for an implementation of objects that have certain attributes and properties in common.
- **Field** - a GraphQL term for an attribute of a node. In this documentation, we will refer to the field as the “object” on which you can query.
- **Object type** - a GraphQL term for an attribute of an object. The attributes of an object depend on the object itself and its relationship to other objects.For the purposes of this documentation, we will refer to the object type as an "attribute" or "attribute of an object." For more information, see [GraphQL Basics](/current/api/metadata_api/en-us/docs/meta_api_examples#GraphQL-basics).

---

# How to Authenticate

There are two primary ways you can use the Tableau Metadata API:

- **Interactively through GraphiQL**. GraphiQL is an in-browser query tool. For more information, see Explore the GraphQL schema using GraphiQL.
- **Programmatically through the GraphQL endpoint**: For this, you need to get an authentication token, as described in the Authenticate using Tableau REST API method.

To access and use the Metadata API in one of the two ways described above, it’s important to know if your Tableau Cloud site or Tableau Server is licensed with Data Management.

- **If licensed with Data Management:** You can start using the GraphiQL tool immediately or review the section below to learn how to get an authentication token so you can programmatically use the Metadata API.
- **If *not* licensed with Data Management:** After the Metadata API has been enabled, review the section below to learn how to get an authentication token so you can programmatically use the Metadata API.

**In this section**

- Authenticate through Sign In method
    - Sign in using a personal access token (PAT)
    - Sign in using a JSON web token (JWT)
    - Sign in using username and password

## Authenticate through Sign In method

The Metadata API requires that you send an authentication token with each query sent. The token lets Tableau Cloud or Tableau Server verify your identity and makes sure that you’re signed in. To get a token, you can call the Sign In method, in one of three ways, using the Tableau REST API versions 3.6 or later.

### Sign in using a personal access token (PAT)

Refer to [Make a Sign In Request with a Personal Access Token [↗]](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_concepts_auth.htm#make-a-sign-in-request-with-a-personal-access-token) in the Tableau REST API Help for more information.

### Sign in using a JSON web token (JWT)

Starting in Tableau Cloud October 2023 / Server 2023.3, the Metadata API respects credentials tokens that were obtained via a JSON web token (JWT) and Tableau connected apps.

Set scope (scp) in the JWT to “tableau:content:read”. The permissions of the user in the JWT determine query results.

Refer to [Make a Sign In Request with a JWT [↗]](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_concepts_auth.htm#make-a-sign-in-request-with-jwt) in the Tableau REST API Help for more information on using a JWT to create a credentials token that you can use with the Metadata API.

### Sign in using username and password

Refer to [Make a Sign In Request with Username and Password [↗]](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_concepts_auth.htm#make-a-sign-in-request-with-username-and-password) in the Tableau REST API Help for more information.

For more information about token expiration, changing the token timeout value, and more, see [Using the Authentication Token In Subsequent Calls [↗]](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_concepts_auth.htm#using_auth_token) topic in the Tableau REST API Help.

---

# Connection Types

Using the Tableau Metadata API, you can access connection information used by Tableau content like data sources, workbooks, and flows.

In some cases you might need to identify the connection type used by the content. When you query a database object or an object that supports the “ConnectionType” property, such as UpstreamDatabases, the query returns the connection type name.

**Note:** CustomSQL database object does not support the ConnectionType property.

**In this section**

- Map ConnectionType names

## Map ConnectionType names

Use the table below to map the ConnectionType name to the connection name that’s displayed in various places on the Tableau platform.

| ConnectionType | Display Name |
|---|---|
| asterncluster | Aster Database |
| athena | Amazon Athena |
| aurora | Amazon Aurora for MySQL |
| awshadoophive | Amazon EMR Hadoop Hive |
| azure_sql_dw | Azure SQL Data Warehouse |
| bigquery | Google BigQuery (ODBC) |
| bigsql | IBM BigInsights |
| box | Box |
| cloudfile:box-excel-direct | Microsoft Excel (Box) |
| cloudfile:box-semistructpassivestore-direct | JSON file (Box) |
| cloudfile:box-textscan | Text file (Box) |
| cloudfile:dropbox-excel-direct | Microsoft Excel (Dropbox) |
| cloudfile:dropbox-semistructpassivestore-direct | JSON file (Dropbox) |
| cloudfile:dropbox-textscan | Text file (Dropbox) |
| cloudfile:googledrive-excel-direct | Microsoft Excel (Google Drive) |
| cloudfile:googledrive-semistructpassivestore-direct | JSON file (Google Drive) |
| cloudfile:googledrive-textscan | Text file (Google Drive) |
| cloudfile:onedrive-excel-direct | Microsoft Excel (OneDrive) |
| cloudfile:onedrive-semistructpassivestore-direct | JSON file (OneDrive) |
| cloudfile:onedrive-textscan | Text file (OneDrive) |
| composite | TIBCO Data Virtualization |
| csv | Text File (legacy) |
| databricks | Databricks |
| dataengine | Tableau Data Engine |
| db2 | IBM DB2 |
| denodo | Denodo |
| denormalized-cube | Denormalized Cube |
| drill | Apache Drill |
| dropbox | Dropbox |
| essbase | Oracle Essbase |
| exasolution | Exasol |
| excel | Microsoft Excel (legacy) |
| excel-direct | Microsoft Excel |
| excel-reader | Microsoft Excel Reader |
| federated | Federated |
| firebird | Firebird |
| genericjdbc | Other Databases (JDBC) |
| genericodbc | Other Databases (ODBC) |
| google-analytics | Google Analytics |
| google-sheets | Google Sheets |
| googlebigquery | Google BigQuery (JDBC) |
| googlecloudsql | Google Cloud SQL |
| googledrive | Google Drive |
| greenplum | Pivotal Greenplum Database |
| hadoophive | Cloudera Hadoop |
| hive | Apache Hive |
| hortonworkshadoophive | Hortonworks Hadoop Hive |
| hyper | Tableau Data Engine |
| inmemfederating | In-memory Federating (multi-connection data source) |
| jdbc | Other Databases (JDBC) Note: This connectionType might display as “unknown.” |
| kognitio | Kognitio |
| maprhadoophive | MapR Hadoop Hive |
| mariadb | MariaDB |
| marklogic | MarkLogic |
| memsql | MemSQL |
| monetdb | MonetDB |
| mongodb | MongoDB BI Connector |
| msaccess | Microsoft Access |
| msolap | Microsoft Analysis Services |
| mysql | MySQL |
| mysql_odbc | unknown |
| netezza | IBM PDA (Netezza) |
| odata | OData |
| odbc | Other Databases (ODBC) Note: This connectionType might display as “unknown.” |
| ogr | Spatial File |
| ogrdirect | Spatial file |
| onedrive | OneDrive |
| oracle | Oracle |
| paraccel | Actian Matrix |
| pdf | PDF file |
| pdf-reader | PDF Reader |
| postgres | PostgreSQL |
| powerpivot | Microsoft PowerPivot |
| presto | Presto |
| progressopenedge | Progress OpenEdge |
| redshift | Amazon Redshift |
| remote-domain | Tableau Server |
| salesforce | Salesforce |
| sapbw | SAP NetWeaver Business Warehouse |
| saphana | SAP HANA |
| semistructpassivestore | JSON file Note: Sometimes this displays as blank. |
| semistructpassivestore-direct | JSON file |
| sharepoint | SharePoint Lists |
| snowflake | Snowflake |
| spark | Spark SQL |
| splunk | Splunk |
| sqlproxy | Tableau Server |
| sqlserver | Microsoft SQL Server |
| stat | Statistical File |
| stat-direct | Statistical file |
| sybasease | SAP Sybase ASE |
| sybaseiq | SAP Sybase IQ |
| tbio | Teradata OLAP Connector |
| teradata | Teradata |
| textclean | Text file |
| textscan | Text file |
| textscan-reader | Text file reader |
| vectorwise | Actian Vector |
| vertica | Vertica |
| vizengine | VizEngine |
| webdata | Web Data Connector |
| webdata-direct | Web Data Connector |
| webdata-direct:anaplan-anaplan | Anaplan |
| webdata-direct:google-ads | Google Ads |
| webdata-direct:intuit-quickbooks | Intuit QuickBooks Online (9.3-2018.1) |
| webdata-direct:intuit-quickbooks-v3 | Intuit QuickBooks Online |
| webdata-direct:marketo-marketo | Marketo |
| webdata-direct:oracle-eloqua | Oracle Eloqua |
| webdata-direct:ServiceNowITSM-ServiceNowITSM | ServiceNow ITSM |

---

# Understand the Metadata Model

At the core of Tableau is data - your data. Your data can come in different formats and structures, categorized at varying levels of detail, and can have relationships with other data. This is the kind of metadata that you can expect to surface from the Metadata API using GraphQL.

To successfully create effective GraphQL queries, you need to understand how Tableau interprets and interacts with content and assets. Understanding this can inform the most efficient way for you to access metadata at the level of detail that you need.

Because the Metadata API uses GraphQL, this section describes the fundamental objects that are available to you to use in a GraphQL query.

**In this section**

- Tableau content and assets
    - Objects and their roles
    - Object types and attributes
    - Other objects related to Tableau content and assets
- Tableau metadata model
    - Example metadata model
    - Example scenario 1: Impact analysis
    - Example scenario 2: Lineage analysis
    - Additional notes about the metadata model

## Tableau content and assets

The Metadata API surfaces the content and assets that comprise your Tableau Cloud site or Tableau Server, the content and asset roles, and how the content and assets relate to each other.

**General Tableau content**

Tableau content is unique to the Tableau platform. Content includes the following:

- Data sources - both published and embedded
- Workbooks
- Sheets
- Dashboards - including stories
- Fields: calculated, column - as they relate to the data source, group, bin, set, hierarchy, combined, and combined set
- Filters: data source
- Parameters
- Flows
- Virtual connections
- Virtual connection tables

**Tableau Cloud and Tableau Server specific content**

Tableau content that can only be managed through Tableau Cloud or Tableau Server includes the following:

- Sites
- Projects
- Users
- Certifications and certifiers
- Data quality warnings and messages

Note: Some content listed above can be managed through the [Tableau REST API [↗]](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_metadata.htm) as well.

**External assets associated with Tableau content**

The Metadata API treats information about any data that comes from outside of the Tableau environment as external assets. External assets include the following:

- Databases - includes local files, remote connections to servers, and web data connectors (WDC)
- Tables - includes queries (custom SQL)

**Note:** Cubes are not supported.

### Objects and their roles

The GraphQL schema used by the Metadata API organizes content and assets on Tableau Cloud and Tableau Server by grouping them by object and role. These objects and their roles are the foundation to your GraphQL queries. At the broadest level, assets on Tableau Cloud can be organized by object and their role. These objects are the foundation to your GraphQL queries.

#### Parent object and role

Parent objects, a mix of both content and assets, can be managed independently of other objects and play the role of a container. Parent objects can refer to other parent objects, can refer to child objects, and can also own child objects.

**Example parent objects**

- Databases
- Tables
- Published data sources
- Virtual connections
- Virtual connection tables
- Workbooks
- Flows
- Sites
- Projects
- Users

#### Child object and role

Child objects, also a mix of both content and assets, cannot be managed independently of their parent object. Therefore, child objects play a dependent role on their parent object.

Some child objects can own other child objects, can refer to other child objects, and in some cases, refer to certain parent objects.

**Example child objects**

- Columns
- Fields
- Embedded data sources
- Sheets
- Parameters
- Data source filters

#### Parent/child object relationship

The GraphQL schema defines the parent/child object relationship by what the objects can contain. In other words, an object is a parent object when it functions as a container for other (child) objects. Some examples of such relationships are below.

**Example parent/child relationships**

- Database
    - DatabaseTable
        - Column
- PublishedDatasource
    - Field
    - DataSourceFilter
    - Parameter
    - DataQualityWarning
    - DataQualityCertification
- Workbook
    - View
    - EmbeddedDatasource
        - Field
    - Parameters

### Object types and attributes

The GraphQL schema organizes objects into interfaces and types.

- An **interface** defines a list of attributes (known as “fields” in GraphQL) that are required for that interface.
- A **type** implements an interface, and may define additional attributes.

In other words, an **interface** serves as template (of sorts) for a **type**, in that it defines a list of attributes that will be common to all types that are based on it. Any **type** that uses the **interface** as a template (we say that “the **type** implements the **interface**”) will contain all the common attributes of the **interface**, plus any other attributes added by the **type**.

For more information on GraphQL interfaces, objects, and fields, see [Schemas and Types [↗]](https://graphql.org/learn/schema/) in the [GraphQL documentation [↗]](http://www.graphql.org).

**Examples of interfaces, types, and attributes**

The table below captures some example interfaces, some example attributes (fields) they must contain, some types that implement that interface, and some attributes that the type adds.

| Database | CertificationNote certifier connectionType contact dataQualityCertifications dataQualityWarning dataQualityWarnings description downstreamDashboards downstreamDatabases downstreamDatasources downstreamFlows downstreamLenses downstreamMetrics downstreamOwners downstreamSheets downstreamTables downstreamVirtualConnections downstreamVirtualConnectionTables downstreamWorkbooks hasActiveWarning id isCertified isControlledPermissionsEnabled isEmbedded isGrouped luid name tags upstreamDatabases upstreamDataQualityWarnings upstreamDatasources upstreamFlows upstreamTables upstreamVirtualConnections upstreamVirtualConnectionTables vizportalId | CloudFile | fileExtension fileId mimeType provider requestUrl |
|---|---|---|---|
| DatabaseServer | extendedConnectionType hostName port service |  |  |
| File | filePath |  |  |
| WebDataConnector | connectorUrl |  |  |
| Table | columns description downstreamDashboards downstreamDatabases downstreamDatasources downstreamFlows downstreamLenses downstreamMetrics downstreamOwners downstreamSheets downstreamTables downstreamVirtualConnections downstreamVirtualConnectionTables downstreamWorkbooks id isEmbedded name upstreamDatabases upstreamDatasources upstreamFlows upstreamTables upstreamVirtualConnections upstreamVirtualConnectionTables | VirtualConnectionTable | containsUnsupportedCustomSql dataQualityCertifications dataQualityWarnings extractLastRefreshedAt extractLastRefreshType hasActiveWarning isCertified isExtracted luid owner tags upstreamDataQualityWarnings uri vizportalId vizportalUrlId |
| DatabaseTable | certificationNote certifier connectionType contact database dataQualityCertifications dataQualityWarning dataQualityWarnings fullName hasActiveWarning isCertified luid schema tags upstreamDataQualityWarnings vizportalId |  |  |
| CustomSQLTable | connectionType database isUnsupportedCustomSql query tables |  |  |

### Other objects related to Tableau content and assets

**Lineage shortcut objects**

The Metadata API enables you to see relationships between the content and asset that you’re evaluating with other items on your Tableau Cloud site or Tableau Server. These items include the following:

- Upstream and downstream content - including data sources, workbooks, sheets, virtual connections, virtual connection tables, fields, flows, and owners
- Upstream and downstream assets - including databases, tables, and columns

You can quickly access this type of information by using *shortcut* objects defined in the GraphQL schema. These lineage or relationship objects use the *upstream* or *downstream* prefix. For example, you can use `upstreamTablesConnection` to query the tables used by a data source, use `downstreamSheetsConnection` to query sheets used by a workbook, or `upstreamLinkedFlows` and `downstreamLinkedFlows` to query flows that are directly upstream or downstream of one another.

For an example query that uses lineage shortcut, see Filtering section in the Example Queries topic. For more information about linked flows, see Working with linked flow objects below.

**Pagination objects**

The Metadata API enables you to traverse through relationships within the data that you’re querying using pagination. The GraphQL schema defines pagination objects as those objects that use the **Connection** suffix. For example, you can use the `databasesConnection` to get a list of paginated results of database assets on your Tableau Cloud site or Tableau Server.

For an example query that uses pagination, see Pagination section in the Example Queries topic.

**Inherited objects**

You can query description attributes of fields using the Metadata API. These description attributes can be inherited from other upstream objects. For example, a field attribute can be inherited from an upstream column in a table or from an upstream data source authored in Tableau Desktop. Beginning in version 2021.2, you can include a description inheritance object, `descriptionInherited`, in your query to return supplemental description information inherited from the closest upstream object.

For an example query that uses the description inheritance object, see Inheritance section in the Examples Queries topic.

## Tableau metadata model

Together, all objects, the roles they play, the relationships they have with each other and their attributes and properties define the metadata model for a particular set of objects. The metadata model is a snapshot (or in GraphQL terms, a graph) of how Tableau interprets and relates a set of objects on your Tableau Cloud site or Tableau Server. From the metadata model, you can understand the dependencies and relationships in your data.

To see how the metadata model used by the Metadata API works, review the following example scenarios.

### Example metadata model

Suppose you have three workbooks (parent objects) and two data sources (parent objects) published to Tableau Cloud or Tableau Server.

For this scenario, the two published data source objects refer to the three workbook objects. The workbook objects own four sheet objects. These sheet objects in turn refer to two field objects that are owned by the published data source objects.

Here’s what the metadata model for this scenario might look like.

**Notes:**

- A sold line with an arrow indicates a ownership relationship.
- A dotted line indicates a reference relationship.

### Example scenario 1: Impact analysis

In an impact analysis scenario, the metadata model can help you answer how data might be affected if a part of the metadata model changes.

In this scenario, you might want to know what could happen if the published data source, *John County - 1*, is deleted.

As you know now from the metadata model, sheet objects are owned by workbook objects, and sheet objects can refer to the field objects. In this scenario, the field objects are owned by the published data source objects. Published data source objects own field objects. Therefore, if *John County - 1* is deleted, the child objects, F1, Hills Library, and Garden Library, are directly affected and their existence compromised because of their dependency on that data source object. The other child objects, F2, Garden Senior Center, and Cliff Senior Center, though they might be affected by the data source object being deleted, their existence is not compromised.

During this analysis, you can see that because *John County - 1* doesn’t own the workbook objects that connect to it, the workbook objects themselves, both *Sakura District* and *Maple District* can continue to exist in the absence of the published data source object.

### Example scenario 2: Lineage analysis

In a lineage flow analysis scenario, you can look at particular part of the metadata model and identify where the data is coming from and how the data reacts or is affected by different parts of the metadata model.

In this scenario, you might want to know where a data point from a sheet object, *Garden Senior Center*, in a workbook object, *Maple District*, is coming from.

Based on what you know from the metadata model, attributes are inherited from the parent object Faye workon this part. Therefore, if you start from *Garden Senior Center* sheet object, you can move to the referring field object, F2, to see that it’s owned by a published data source object. In this case, *John County - 2* is the source for the data point.

During this analysis, you are able to freely include or exclude parts of the metadata model in order to understand the lineage flow for *Garden Senior Center*. For example, you can choose to exclude *Garden Library* sheet object even though it’s also owned by the same workbook object, *Maple District*.

### Additional notes about the metadata model

**Using custom SQL**

The metadata model interprets customSQL queries as tables.

When custom SQL queries are defined in your data source or flow, the queries have to fit a set of criteria to be recognized and interpreted by the Metadata API. For more information, see [Tableau Catalog support for custom SQL [↗]](https://help.tableau.com/current/pro/desktop/en-us/customsql.htm#tableau-catalog-support-for-custom-sql) in the Tableau Help.

For programmatic methods to determine if the custom SQL used in a data source or flow is unsupported, see the isUnsupportedCustomSql and containsUnsupportedCustomSql booleans in the API reference.

**Using the databaseTable object in a query**

When you run a query using the [databaseTable](https://help.tableau.com/current/api/metadata_api/en-us/reference/databasetable.doc.html) object, for some databases, the schema attribute might not return the correct schema name for the table. This issue can occur when the selected schema, while creating or editing a data source, workbook, or flow, is changed after adding the table.

When the selected schema changes after adding the table, the schema attribute your query returns is the name of the last selected schema instead of the actual schema that the table is using.

Databases that might return the incorrect schema in the scenario described above include Amazon Athena and Exasol.

**When workbook lineage query results are missing upstream databases**

When you run a lineage query to determine upstream databases (D1 and D2) from a workbook (WB), only the databases (D1) whose fields (F1) are used in a workbook’s sheet (S) are returned. Databases (D2) that might be referenced by the workbook, but whose fields aren’t used by a sheet directly, won’t show as upstream.

However, all databases (DB1 and DB2) can show as upstream if you run a lineage query that starts from the data source used by the workbook, for example.

**Running a tags query**

Unless you’re a Tableau Cloud site or Tableau Server admin, by default, results from a tags query that uses the asset attribute are obfuscated to block all sensitive data. Results are always obfuscated, whether you have the appropriate permissions to see the associated object’s metadata or not. To see the associated object’s metadata that you have the appropriate permissions to see, you can replace the asset attribute with a type-specific attribute instead.

For example, suppose you have permissions to see WorkbookA. You run the following tags query, which uses the asset attribute to return all objects that use tags.

```
{
  tags {
    assets {
      id
      name
    }
  }
}
```

In this example, the query returns two workbooks with its results obfuscated.

```
{
  tags {
    assets {
      { id: "A", name: null }
      { id: "B", name: null }
    }
  }
}
```

Because results are always obfuscated when you use the asset attribute in a tags query, you can modify the query to be type specific to return the results you expect. In this example, you can run a query using the workbook type instead.

```
{
  tags {
    workbooks {
      id
      name
    }
}
```

When you modify the tags query to be type specific, the query returns the tags metadata for WorkbookA.

```
{
  tags {
    workbooks {
      { id: "A", name: "WorkbookA" }
      { id: "B", name: null }
    }
}
```

**Working with linked flow objects**

Unlike `upstreamFlows` and `downstreamFlows` objects, which return all upstream and downstream flows, queries that include `upstreamLinkedFlows` and `downstreamLinkedFlows` objects return additional structural metadata about upstream and downstream flows. The `upstreamLinkedFlows` and `downstreamLinkedFlows` objects can provide information about order, distance, and relationship between flows that are directly upstream and downstream of one another.

For example, suppose you have a flow called Superstore. You can run the following query to compare the information returned for the flows upstream of the Superstore flow.

```
query upstreamflows_vs_upstreamlinkedflows {
  flows(filter: {name:"Superstore_AllUp"}) {
    name
    id
    upstreamFlows{
      name
      id
    }
    upstreamLinkedFlows{
      asset{
        name
        id
      }
      toEdges
      fromEdges
    }
  }
}
```

The above query returns the following information.

```
{
  "data": {
    "flows": [
      {
        "name": "Superstore_Customers",
        "id": "089839c4-7edd-b887-3178-fcc2367e938b",
        "upstreamFlows": [
          {
            "name": "Superstore_Pods",
            "id": "66e8f90d-f02d-eea1-850e-2014b4b09b96"
          },
          {
            "name": "Superstore_Marketing",
            "id": "7efe5f86-511b-760b-18d4-1a6a28c220b5"
          },
          {
            "name": "Superstore_Operations",
            "id": "cd79250f-bc27-1d9d-7b22-aca1534863e1"
          },
          {
            "name": "Superstore_Sales",
            "id": "e78d4c54-df3a-12a9-ef1a-9849c2625b35"
          }
        ],
        "upstreamLinkedFlows": [
          {
            "asset": {
              "name": "Superstore_Customers",
              "id": "089839c4-7edd-b887-3178-fcc2367e938b"
            },
            "toEdges": [
              "e78d4c54-df3a-12a9-ef1a-9849c2625b35",
              "7efe5f86-511b-760b-18d4-1a6a28c220b5",
              "cd79250f-bc27-1d9d-7b22-aca1534863e1"
            ],
            "fromEdges": []
          },
          {
            "asset": {
              "name": "Superstore_Sales",
              "id": "e78d4c54-df3a-12a9-ef1a-9849c2625b35"
            },
            "toEdges": [
              "cd79250f-bc27-1d9d-7b22-aca1534863e1"
            ],
            "fromEdges": [
              "089839c4-7edd-b887-3178-fcc2367e938b"
            ]
          },
          {
            "asset": {
              "name": "Superstore_Marketing",
              "id": "7efe5f86-511b-760b-18d4-1a6a28c220b5"
            },
            "toEdges": [],
            "fromEdges": [
              "089839c4-7edd-b887-3178-fcc2367e938b"
            ]
          },
          {
            "asset": {
              "name": "Superstore_Operations",
              "id": "cd79250f-bc27-1d9d-7b22-aca1534863e1"
            },
            "toEdges": [
              "66e8f90d-f02d-eea1-850e-2014b4b09b96"
            ],
            "fromEdges": [
              "089839c4-7edd-b887-3178-fcc2367e938b",
              "e78d4c54-df3a-12a9-ef1a-9849c2625b35"
            ]
          },
          {
            "asset": {
              "name": "Superstore_Pods",
              "id": "66e8f90d-f02d-eea1-850e-2014b4b09b96"
            },
            "toEdges": [],
            "fromEdges": [
              "cd79250f-bc27-1d9d-7b22-aca1534863e1"
            ]
          }
        ]
      }
    ]
  },
}
```

- From the upstreamFlows object, the following upstream flows are returned: Superstore_Pods, Superstore_Marketing, Superstore_Operations, and Superstore_Sales.
- Whereas, from the upstreamLinkedFlows object, the following information is returned:
    - From the `asset` parameter, you see Superstore_Customers is returned. This is the source flow or the flow from which you want to understand its relationship to other flows.
    - From the `fromEdges` parameter, you see three direct flows that Superstore_Customer links from: Superstore_Marketing, Superstore_Operations, and Superstore_Sales. In addition, you see additional flow relationships information about those three direct flows.
        - Superstore_Marketing, `fromEdges` parameter shows Superstore_Customers as its direct upstream flow and the `toEdges` parameter shows no direct downstream flows.
        - Superstore_Operations, `toEdges` shows both Superstore_Customers and Superstore_Sales as its direct upstream flows and `toEdges` shows Superstore_Pods directly downstream of it.
    - From the `toEdges` parameter, you see that Superstore_Customers has no direct flows it links to. In other words, Superstore_Customers is at the bottom of its relationship chain.

---

# How Permissions Work

Tableau Cloud and Tableau Server provide a space for accessing and managing published content. Beginning with version 2019.3, using the Tableau Metadata API, you have the ability to track and manage metadata and lineage of external assets used by the content published to your Tableau Cloud site or Tableau Server.

In addition, when your Tableau Cloud site or Tableau Server is licensed with Data Management, you have access to features like data quality warnings, which are enabled by [Tableau Catalog [↗]](https://help.tableau.com/current/server/en-us/dm_catalog_overview.htm).

**In this section**

- Tableau Catalog indexes content, assets, and metadata
- Permissions on assets and their metadata
    - Access metadata about content and assets

## Tableau Catalog indexes content, assets, and metadata

Metadata API contains indexed content, assets, and metadata from the content that has been published to your Tableau Cloud site or Tableau Server. You can query the Metadata API for all content and assets that it indexes.

The Metadata API exposes metadata for the following:

- **Tableau content**: workbooks, data sources, virtual connections, virtual connection tables, flows, projects, users, and sites.
- **External assets**: databases and tables associated with Tableau content Metadata API classifies the metadata of any data that comes from outside the Tableau environment as external assets. The data that comes from outside the Tableau environment can be stored in many different formats, such as a database server or local .json files. The Metadata API does not track the underlying data in any form (raw or aggregated).

Metadata includes the following:

- **Lineage information** or the relationship between items. For example, the Sales table is used by Superstore data source and Superstore Sample workbook.
- **Schema information**. Some examples include:
    - Table names, column names, and column types. For example, Table A contains Columns A, B, and C, which are types INT, VARCHAR, and VARCHAR.
    - Database name and server location. For example, Database_1 is a SQL Server database at http://example.net.
    - Data source name, and the names and types of the fields the data source contains. For example, Superstore data source has fields AA, BB, and CC. Field CC is a calculated field that refers back to both field AA and field BB.
- **User curated, added, or managed information**. For example, asset descriptions, certifications, user contacts, data quality warnings, and more.

## Permissions on assets and their metadata

Permissions control who is allowed to see and manage the data that is accessible from the Metadata API, for example who can see and manage external assets or who can see relationships shown through lineage queries.

### Access metadata about content and assets

The permissions used to access metadata through the Metadata API work similarly to permissions for accessing content through Tableau Cloud or Tableau Server, with some additional considerations for external assets.

The Metadata API uses the same View capability that Tableau Server uses to control the information you can see, with one fundamental difference. In general, when you don’t have View capabilities to access information, Tableau Server omits (also called filters) that information from your results. However, when you don’t have View capabilities to access information, the Metadata API by default hides (also called obfuscates) that information from your results.

If you would rather omit any detail about the related assets that you do not have View capability to access, you can specify the “filter” mode in a query. If you specify the filter permissions mode, only the resultsor external assets whose attributes of external assets you have permissions to see are returned.

For an example, see Filter mode section of the Example Queries topic.

For more information, see one of the following topics:

- For Tableau Cloud: [Permissions on metadata [↗]](https://help.tableau.com/current/online/en-us/dm_perms_assets.htm#permissions-on-metadata)
- For Tableau Server: [Permissions on metadata [↗]](https://help.tableau.com/current/server/en-us/dm_perms_assets.htm#permissions-on-metadata) Permissions on Tableau content As mentioned above, the Metadata API uses view and manage capabilities that are already used by existing Tableau content to control the information you can see and manage on Tableau content.

For more general information on View capabilities, see one of the following topics:

- For Tableau Cloud: [How Permissions are Evaluated [↗]](https://help.tableau.com/current/online/en-us/permissions.htm#permissioncapabilities)
- For Tableau Server: [How Permissions are Evaluated [↗]](https://help.tableau.com/current/server/en-us/permissions.htm#permissioncapabilities)

#### Permissions on external assets using derived permissions

You are automatically granted View capability to external assets when the derived permissions site setting has been turned on for a site. This site setting is enabled by default if Tableau Cloud or Tableau Server is licensed with Data Management. Without Data Management, the site setting must be enabled manually for Tableau Server by the Tableau Server admin.

In addition, if your site is licensed with Data Management, it’s possible to explicitly grant permissions for external assets.

For more information and details around permissions, see one of the following topics:

- For Tableau Cloud: [Permissions on external assets using derived permissions [↗]](https://help.tableau.com/current/online/en-us/dm_perms_assets.htm#derived)
- For Tableau Server: [Permissions on external assets using derived permissions [↗]](https://help.tableau.com/current/server/en-us/dm_perms_assets.htm#derived)

**Note:** If you are the owner of a flow, derived permissions enable Overwrite and Set Permissions capabilities as well. You can edit and manage permissions for the database and table metadata used by the flow output. For these flow scenarios, the capabilities apply only after there has been at least one successful flow run under you as the current owner of the flow.

---

# Common Errors

If a query encounters an error, the query results return an error name and description instead of query results.

**In this section**

- Common errors
- Other errors

## Common errors

The following errors can occur in your Metadata API query.

| Error | Error Type | Details |
|---|---|---|
| ACCESS_DENIED | Error | You are not authorized to use the Metadata API. |
| BACKFILL_RUNNING | Warning | Still creating the Metadata API Store. Results from the query may be incomplete at this time. |
| FEATURE_DISABLED | Error | Can’t run the query because the Metadata API has not been enabled yet. Run the ‘tsm maintenance metadata-services enable’ command to enable the Metadata API or contact your Tableau administrator. |
| FILTER_REQUIRED | Error | Can’t run the query because you must be an admin. Alternatively, if you are not an admin, you can rerun the query with a field name filter to return results. |
| INHERITANCE_INCOMPLETE | Warning | When queries run in filter mode, inheritance results might be incomplete. |
| INVALID_ARGUMENT | Error | The query contains an undefined fragment ‘n’. Make sure to define the fragment. |
|  | Error | Can’t use nested fragments for shortcut query ‘n’. |
|  | Error | Can’t use “after” and “offset” pagination at the same time. Remove one of the arguments from the query. |
|  | Error | The query contains an invalid cursor. Make sure to use cursors of the same type. |
|  | Error | The query contains an invalid cursor. Make sure to use cursors that belong to the same site. |
|  | Error | A text filter can’t contain only the following characters: spaces or a single quotation mark. Replace the invalid characters and try again. |
|  | Error | Make sure the query contains the field you want to paginate on. |
| LINKED_RESULTS_INCOMPLETE | Warning | When queries run in filter mode, linked results might be incomplete. |
| MAX_PAGE_SIZE_EXCEEDED | Warning | The value chosen for “first” exceeded the maximum page size, so the query results will be limited to the maximum page size instead of the value chosen for “first”. It is recommended that the value chosen for “first” be smaller than the maximum page size, or incomplete results may be returned. |
| NODE_LIMIT_EXCEEDED | Warning | Showing partial results. The request exceeded the ‘n’ node limit. Use pagination, additional filtering, or both in the query to adjust results. |
| PERMISSIONS_MODE_SWITCHED | Error | One or more of attributes used in your filter contain sensitive data. Your results have been automatically filtered to contain only the results you have permissions to see. |
| RATE_EXCEEDED | Error | The rate of API requests exceeded what’s allowed. On Tableau Cloud, the query throttling configuration can’t be changed. On Tableau Server, if Metadata API users are seeing frequent RATE_EXCEEDED errors, a Tableau Server administrator can adjust or disable throttling. See the [metadata.query.throttling.enabled [↗]](https://help.tableau.com/current/server/en-us/cli_configuration-set_tsm.htm#metadata_query_throttling_enabled), [metadata.query.throttling.queryCostCapacity [↗]](https://help.tableau.com/current/server/en-us/cli_configuration-set_tsm.htm#metadata_query_throttling_queryCostCapacity), and [metadata.query.throttling.tokenRefilledPerSecond [↗]](https://help.tableau.com/current/server/en-us/cli_configuration-set_tsm.htm#metadata_query_throttling_tokenRefilledPerSecond) settings for more information. |
| SITE_DISABLED | Error | The Metadata API is not enabled for this site: ‘n’. |
| TIME_LIMIT_EXCEEDED | Warning | Can’t show all data because the timeout limit ‘n’ has been exceeded. Use pagination, additional filtering, or both in the query, and try again. |
| USER_VISIBILITY_IS_LIMITED | Warning | The attributes used in your sort contain sensitive data so results exclude the sort. |

## Other errors

#### Some edge names do not exist on CanHaveLabels interface

Starting with Tableau Cloud March 2023 and Tableau Server 2023.1, DataQualityWarning.asset and DataQualityCertification.asset point to the CanHaveLabels interface instead of the Warnable and Certifiable interfaces. Though they share many of the same edge names, some edge names available on the Warnable and Certifiable interfaces do not exist on the CanHaveLabels interface, and queries that use those edges will now produce an error similar to the following:

> `Validation error of type FieldUndefined: Field 'dataQualityWarnings' in type` `'CanHaveLabels' is undefined @ 'dataQualityWarnings/asset/dataQualityWarnings'`

The following edges will produce an error like the above if referenced through DataQualityWarning.asset or DataQualityCertification.asset:

- hasActiveWarning
- dataQualityWarnings
- dataQualityWarningsConnection
- isCertified
- dataQualityCertifications
- dataQualityCertificationsConnection

**Example query that produces an error:**

```
query myQuery {
  dataQualityWarnings {
    asset {
      dataQualityWarnings {
        message
      }
    }
  }
}
```

The above query produces an error because the dataQualityWarnings.asset (which points to the CanHaveLabels interface) does not have a field named dataQualityWarnings. Before Tableau Cloud March 2023 and Tableau Server 2023.1, the query did not produce an error because dataQualityWarnings.asset pointed to the Warnable interface, and that interface did have a field named dataQualityWarnings.

The query should be adjusted to either:

- Resolution 1: Use the Labels type
- Resolution 2: use an [inline fragment [↗]](https://graphql.org/learn/queries/#inline-fragments) to match Warnable or Certifiable as required.

**Resolution 1: Use the labels edge instead of the dataQualityWarnings edge**

```
query myQuery {
  dataQualityWarnings {
    asset {
      labels {
        message
      }
    }
  }
}
```

**Resolution 2: Use an [inline fragment [↗]](https://graphql.org/learn/queries/#inline-fragments) to limit the interfaces to Warnable**

```
query myQuery {
  dataQualityWarnings {
    asset {
      ... on Warnable {
        dataQualityWarnings {
          message
        }
      }
    }
  }
}
```

---

# Example Queries

**In this section**

- GraphQL basics
- Common queries to get you started
    - What types of database and table assets are used on my site?
    - Find a table
    - Find a table in a workbook using query variables
    - Filtering
    - Pagination
    - Inheritance
    - Permissions
    - Linked flows

## GraphQL basics

If you’re new to GraphQL, check out the many resources to learn GraphQL. For example, [Introduction to GraphQL [↗]](https://graphql.org/learn). To get you started, here are some common GraphQL query-related concepts you should understand.

#### Schema

The GraphQL schema tells you what queries and mutations are allowed. The schema defines the objects, object types, and attributes that you can get from the Tableau Metadata API.

#### Object

The building block of a query is the object. Objects are a collection of data assets accessible from the Metadata API. Objects have object types and attributes.

#### Queries

GraphQL operations come in the form of queries. Queries are also objects, specifically root objects in GraphQL. However, queries are special objects because they define the entry point of every GraphQL query.

For example, suppose you have the following GraphQL query:

```
query {
    hello
}
```

The operation above begins with the required `query` root object. The object being requested is `hello`. The result of the query is:

```
"data": {
    "hello": "world"
},
```

As you can see, GraphQL returns data in a similar shape as the query itself.

## Common queries to get you started

After identifying the data that you’re interested in, you can write a query to access the information.

### What types of database and table assets are used on my site?

To answer what types of databases and tables are used on your site, you might use the following query:

```
query getDatabasesOnMySite{
    databases {
        id
        name
    }
}
query getTablesTablesOnMySite{
    tables {
        id
        name
    }
}
```

Here, `databases` and `tables` are objects and `id` and `name` are attributes in the Metadata API that you can query.

### Find a table

To find a specific table, such as the *Orders* table, you can pass a filter parameter with a value of “Orders” through the `tables` object. Your query might look like the following:

```
query ShowMeOneTableCalledOrders{
  tables (filter: {name: "Orders"}){
    name
    columns {
      name
    }
  }
}
```

Here, `tables` is the object and `name` and `columns` are attributes in the Metadata API that you can query. The filter parameter that you pass through the `table` object uses the format `(filter: {name: "<name-of-the-table>"})`.

### Find a table in a workbook using query variables

To find a specific table in a workbook using a dynamic value instead of a static value in the filter parameter, you can use a query variable. A query variable is value that you can define outside of the query so that it can be changed dynamically. This query variable is a second object that’s passed through the same query.

The query variable uses the following formats depending on where the query variable is being used:

- When defining the variable name: `variableName: <value>`
- Declaring the variable in the query: `{name: $variableName}`

For example, suppose you want to use a query variable to find the *Sample_Superstore* workbook. You might use the following query, where the query variable is “$workbookname” and the value for the variable in this example is “Sample_Superstore”.

```
query ShowMeAnyTable($workbookName: String, $sheetName: String){
  workbooks(filter: {name: $workbookName}){
    name
    embeddedDataSources {
      fields {
        name
        ...on CalculatedField {
          formula
        }
      }
    }
  sheets(filter: {name: $sheetName}){
    name
    worksheetFields {
      name
      formula
      }
    }
  }
}
```

### Filtering

A filter sorts through and retrieves only the data that meets a set of specified criteria.

For example, suppose you’re looking for a published data source named “Orders.” You might use the following query to find information about the published data source:

```
query FilteredQuery {
  publishedDatasources(filter: {name: "Orders"}) {
    name
    hasExtracts
    upstreamTables
  }
}
```

Here, the filter parameter that you pass through the `publishedDataSources` object uses the format `(filter: {name: "<name-of-the-data-source>"})`.

Suppose you’re looking for multiple published data sources, “Orders” and “Education.” You might use the following query to find information about these published data sources:

```
query FilteredWithinQuery {
  publishedDatasources(filter: {nameWithin: ["Orders", "Education"]}) {
    name
    hasExtracts
    upstreamTables
  }
}
```

To filter for multiple published data sources, the filter parameter that you pass through the `publishedDataSources` object uses the format `(filter: {namewithin: ["<name-of-the-data-source1>", "<name-of-the-data-source2>"]})`.

### Pagination

Pagination enables you to traverse through relationships among the data that you’re querying. The Metadata API defines pagination objects as those that have “Connection” appended to it.

There are two common methods for pagination that these objects can support: offset and cursors. The maximum page size using either method is 1,000 items.

#### Paginate using offset method

Using the offset method, you can specify the number of items you want and where to start counting the items you want retrieved. This method allows for larger queries. However, it also might affect performance of the query as the query gets larger.how to do you know how many items you want

For example, suppose you want to retrieve data about published data sources. In the `publishedDataSourcesConnections` object parameter you can specify the number of items you want using “first” and where to start counting the items you want retrieved using “offset”.

The following query gives you the ***first*** ten items.

```
query PaginatedQueryOffsetPage1 {
  publishedDatasourcesConnection(first: 10, offset: 0) {
    nodes {
      name
    }
  }
}
```

Here, the pagination format you can use for offset is `(first: <value>, offset: <value>)`.

The following query gives you the ***next*** ten items, for a total of 20 items.

```
query PaginatedQueryOffsetPage2 {
  publishedDatasourcesConnection(first: 10, offset: 10) {
    nodes {
      name
    }
  }
}
```

#### Paginate using cursor method

Using cursors, you can retrieve data by specifying the page size and then using a “cursor” to keep track of where the next set of data should be retrieved from. This method is faster than the offset method when you need return results for large lists.

For example, suppose you want to retrieve data about published data sources. In the publishedDataSourcesConnection object parameter, you can specify the number of items you want using “first”.

Then, instead of counting pages using “offset” like in the offset method, you pass in “after” when using the cursor method, which is a value that is returned by the pageInfo attribute in the publishedDataSourcesConnection object.

```
query PaginatedQueryCursorPage1 {
  publishedDatasourcesConnection(first: 10, orderBy: {field: NAME, direction: ASC}) {
    nodes {
      name
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}
```

**Notes:**

- hasNextPage is only true if there are more pages after given OrderBy and first parameters.
- endCursor should be saved and passed to the query for page two.

When you include “after” in the parameter, the value of the “endCursor” is passed into a subsequent query.

```
query PaginatedQueryCursorPage2 {
  publishedDatasourcesConnection(first: 10, after: null, orderBy: {field: NAME, direction: ASC}) {
    nodes {
      name
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}
```

### Inheritance

When querying fields, you can include a description inheritance object, `descriptionInherited`, in your query to return information about the closest upstream object where the description is inherited from. Including this description inheritance object in your query can be useful for providing supplemental description information associated with upstream objects.

The following query returns description attribute information for fields in all data sources that are associated with the specified table.

```
query whereIsThisDescriptionComingFrom {
  databaseTables(filter: {id: "57a8c84f-0671-4df5-964b-774fc68968b1"}) {
    downstreamDatasources {
      fields {
        description
        descriptionInherited {
          value
          asset {
            id
            __typename
          }
        }
      }
    }
  }
}
```

### Permissions

There are two permission modes available in the Metadata API:

- Obfuscate
- Filter

#### Obfuscate mode

If you do not specify a permissions mode in a query, the default permission mode, obfuscate, is applied. Obfuscate returns all results, but will hide any sensitive attributes on external assets that you do not have permissions to see. For more information, see Access Metadata about Related Asset section of the How Permissions Work topic.

You can explicitly specify the obfuscate permissions mode by passing the permissions mode parameter through the object using the format `(permissionMode: OBFUSCATE_RESULTS)` like in the example below.

```
query ObfuscatePermissionsQuery {
  databases(permissionMode: OBFUSCATE_RESULTS) {
    tables{
      downstreamDatasources{
        name
      }
    }
  }
}
```

In the example query above, although you might not have permissions to all the databases, the obfuscate permissions mode returns all the related data sources that you have permissions to see.

#### Filter mode

If you specify the filter permissions mode, only the resultsor external assets whose attributes of external assets you have permissions to see are returned.

You can specify the filter permissions mode by passing the permissions mode parameter through the object using the format `(permissionMode: FILTER_RESULTS)` like in the example below.

```
query FilteredPermissionsQuery {
  databases(permissionMode: FILTER_RESULTS) {
    tables{
      downstreamDatasources{
        name
      }
    }
  }
}
```

In the example query above, because you might not have permissions to all the database, the filter permissions mode returns only the data sources that are related to the databases and tables that you have permissions to see. This means, a query like this might omit data sources that you might have permissions to see.

### Linked flows

You can use linked flow objects in a query to return information about which flows are directly upstream or downstream of one another.

The following query returns all the downstream flow objects that have a relationship to Flow1.

```
query flows {
  flows (filter: {name: "Flow1"}){
    name
    downstreamLinkedFlows{
        asset{
          name
          id
      }
      toEdges
      fromEdges
    }
  }
}
```

For more information about linked flow objects, see Working with linked flow objects.

---

# Sample Scripts

To help you use the Tableau Metadata API, Tableau provides sample scripts that you can download and modify for your purposes.

The sample scripts can be downloaded from GitHub: [https://github.com/tableau/metadata-api-samples [↗]](https://github.com/tableau/metadata-api-samples).

You can also download the Metadata API schema files from here:

- schema.json
- schema.graphql

---

# Release Notes

Significant changes to this project are noted in this document.

**Jump to version**

- Tableau Cloud June 2024 / Server 2024.2
- Tableau Cloud February 2024
- Tableau Cloud October 2023 / Server 2023.3
- Tableau Cloud June 2023
- Tableau Cloud March 2023 / Server 2023.1
- Tableau Cloud December 2022
- Tableau Cloud October 2022 / Server 2022.3
- Tableau Cloud June 2022
- Tableau Cloud March 2022 / Server 2022.1
- Tableau Cloud December 2021 / Server 2021.4
- Older versions

### Tableau Cloud June 2024 / Server 2024.2

#### Added

- Added Tableau Pulse MetricDefinition objects and supporting shortcuts to the schema.
- Tableau Server: Lens objects and supporting shortcuts are retired. No lens or lens properties are returned from queries. (Removed from Tableau Server in this version. Already removed from Tableau Cloud in February 2023.)
- Tableau Server: Metrics objects and supporting shortcuts are retired. No metrics or metrics properties are returned from queries. (Removed from Tableau Server in this version. Already removed from Tableau Cloud in February 2023.)

### Tableau Cloud February 2024

#### Retired

- Lens objects and supporting shortcuts are retired. No lens or lens properties are returned from queries.
- Metrics objects and supporting shortcuts are retired. No metrics or metrics properties are returned from queries.

### Tableau Cloud October 2023 / Server 2023.3

#### Added

- Tableau Server: Added Metadata API request throttling. Requests that exceed the rate threshold receive a RATE_EXCEEDED error. (New for Tableau Server in this version. Already added to Tableau Cloud in June 2023.)
- The Metadata API now respects credentials tokens that were obtained via a JSON web token (JWT) and Tableau connected apps. You still use the REST API Sign In method to get a credentials token, but now you can use a JWT in the REST API Sign In request. For information, see Sign in using a JSON web token (JWT).

### Tableau Cloud June 2023

#### Added

- Added Metadata API request throttling. Requests that exceed the rate threshold receive a RATE_EXCEEDED error.
- Types that implement the Field interface can be filtered by name using a text filter. The filter will return fields where any word in the field name begins with the specified case-insensitive text.

Example:

```
query FilteredFields {
    publishedDatasources {
        name
        fields (filter: {text: "age"}) {
            id
            name
        }
    }
}
```

### Tableau Cloud March 2023 / Server 2023.1

#### Changed

- The virtualConnection object’s connectionType attribute now returns ‘null’, and may be removed in a future release. Use ‘connectionType’ of ‘upstreamDatabases’ instead. (New for Tableau Server 2023.1. Changed in Tableau Cloud in December 2022)
- The DataQualityWarning.asset and DataQualityCertification.asset edges now point to the CanHaveLabels interface instead of the Warnable and Certifiable interfaces. This change was made to move the schema towards a more general interface that works with all labels. Referencing edge names that exist on the Warnable and Certificable interfaces but do not exist on the CanHaveLabels interface will produce an error. For information on the error and adjusting queries that may be affected, see “Other Errors”, in the Common Errors topic. The list of affected edges is:
    - Warnable.hasActiveWarning
    - Warnable.dataQualityWarnings
    - Warnable.dataQualityWarningsConnection
    - Certifiable.isCertified
    - Certifiable.dataQualityCertifications
    - Certifiable.dataQualityCertificationsConnection

### Tableau Cloud December 2022

#### Changed

- The virtualConnection object’s connectionType attribute now returns ‘null’, and may be removed in a future release. Use ‘connectionType’ of ‘upstreamDatabases’ instead.

### Tableau Cloud October 2022 / Server 2022.3

#### Added

- Modified existing data quality warning shortcuts to support data quality warnings on columns.

### Tableau Cloud June 2022

#### Added

- Added lens objects and supporting shortcuts to the schema.
- Added new fields to existing objects: Object New fields tableauSites createdAt workbooks projectLuid datasources createdAt, updatedAt flows createdAt, updatedAt virtualConnections createdAt, updatedAt

### Tableau Cloud March 2022 / Server 2022.1

#### Added

- Added the virtualConnection and virtualConnectionTable objects and supporting shortcuts to the schema.

### Tableau Cloud December 2021 / Server 2021.4

#### Changed

- Some shortcuts were changed to make the definitions of “upstream” and “downstream” more consistent throughout the schema.
    - The `Column` type’s `upstreamTables` and `downstreamTables` shortcuts no longer include the table that contains the column. Use `Column.table` to find information about the containing table instead.
    - The `Table` type’s `upstreamDatabases` shortcut no longer includes the database that contains the table. Use `... on DatabaseTable {database}` instead.
    - The `Workbook` type’s `downstreamOwners` field no longer includes the owner of the workbook. Use `Workbook.owner` to find information about the workbook owner instead.

### Older versions

The Metadata API was officially released in Tableau Server 2019.3 and Tableau Cloud in September of 2019. This document tracks Metadata API changes starting with 2021.4.

---

# About Tableau Help

### Addressing Implicit Bias in Technical Language

In an effort to align with one of our core company values, equality, we have changed terminology to be more inclusive where possible. Because changing terms in code can break current implementations, we maintain the current terminology in the following places:

- Tableau APIs: methods, parameters, and variables
- Tableau CLIs: commands and options
- Installers, installation directories, and terms in configuration files
- Tableau Resource Monitoring Tool (we plan to make changes to non-inclusive terminology in the web interface, error messages, and related documentation soon.)
- Third-party systems documentation

For more information about our ongoing effort to address implicit bias, see [Salesforce Updates Technical Language in Ongoing Effort to Address Implicit Bias [↗]](https://www.salesforce.com/news/stories/salesforce-updates-technical-language-in-ongoing-effort-to-address-implicit-bias) on the Salesforce website.
