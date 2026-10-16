# Orders service

Run tests with `make test`. The API listens on port 8080 in development.

## Background

REST stands for Representational State Transfer. It is an architectural style for networked applications in which
a client talks to a server through a small set of operations on resources. A resource is anything the server can name,
such as a user, an order or an invoice, and each resource is addressed by a URL. The client reads a resource with GET,
creates one with POST, replaces one with PUT, changes part of one with PATCH and removes one with DELETE. Servers are
stateless, which means that every request carries everything the server needs to answer it, so that any server instance
can handle any request. Responses describe whether the request worked through a status code: codes in the 200 range mean
success, codes in the 400 range mean the client made a mistake and codes in the 500 range mean the server failed.
Representations of resources are usually JSON. Caching is possible because GET requests must not change state. Because
of these properties REST services scale well and are easy to understand, which is why most web APIs use the style.
Pagination is commonly done with query parameters. Versioning is commonly done with a prefix in the URL path.

## Layout

Handlers live in `handlers/`, storage in `store/`.
