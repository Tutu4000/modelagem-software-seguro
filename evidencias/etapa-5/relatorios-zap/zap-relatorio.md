# ZAP Scanning Report

ZAP by [Checkmarx](https://checkmarx.com/).


## Summary of Alerts

| Risk Level | Number of Alerts |
| --- | --- |
| High | 0 |
| Medium | 0 |
| Low | 4 |
| Informational | 2 |




## Alerts

| Name | Risk Level | Number of Instances |
| --- | --- | --- |
| Server Leaks Version Information via "Server" HTTP Response Header Field | Low | 3 |
| Strict-Transport-Security Header Not Set | Low | 3 |
| Unexpected Content-Type was returned | Low | 15 |
| ZAP is Out of Date | Low | 1 |
| A Client Error response code was returned by the server | Informational | 25 |
| Non-Storable Content | Informational | 3 |




## Alert Detail



### [ Server Leaks Version Information via "Server" HTTP Response Header Field ](https://www.zaproxy.org/docs/alerts/10036/)



##### Low (High)

### Description

The web/application server is leaking version information via the "Server" HTTP response header. Access to such information may facilitate attackers identifying other vulnerabilities your web/application server is subject to.

* URL: https://nginx:8443/api/v1/arquivos/id

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `nginx/1.30.6`
  * Other Info: ``
* URL: https://nginx:8443/api/v1/arquivos/id/download

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `nginx/1.30.6`
  * Other Info: ``
* URL: https://nginx:8443/api/v1/arquivos

  * Method: `POST`
  * Parameter: ``
  * Attack: ``
  * Evidence: `nginx/1.30.6`
  * Other Info: ``


Instances: 3

### Solution

Ensure that your web server, application server, load balancer, etc. is configured to suppress the "Server" header or provide generic details.

### Reference


* [ https://httpd.apache.org/docs/current/mod/core.html#servertokens ](https://httpd.apache.org/docs/current/mod/core.html#servertokens)
* [ https://learn.microsoft.com/en-us/previous-versions/msp-n-p/ff648552(v=pandp.10) ](https://learn.microsoft.com/en-us/previous-versions/msp-n-p/ff648552(v=pandp.10))
* [ https://www.troyhunt.com/shhh-dont-let-your-response-headers/ ](https://www.troyhunt.com/shhh-dont-let-your-response-headers/)


#### CWE Id: [ 497 ](https://cwe.mitre.org/data/definitions/497.html)


#### WASC Id: 13

#### Source ID: 3

### [ Strict-Transport-Security Header Not Set ](https://www.zaproxy.org/docs/alerts/10035/)



##### Low (High)

### Description

HTTP Strict Transport Security (HSTS) is a web security policy mechanism whereby a web server declares that complying user agents (such as a web browser) are to interact with it using only secure HTTPS connections (i.e. HTTP layered over TLS/SSL). HSTS is an IETF standards track protocol and is specified in RFC 6797.

* URL: https://nginx:8443/api/v1/arquivos/id

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: ``
  * Other Info: ``
* URL: https://nginx:8443/api/v1/arquivos/id/download

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: ``
  * Other Info: ``
* URL: https://nginx:8443/api/v1/arquivos

  * Method: `POST`
  * Parameter: ``
  * Attack: ``
  * Evidence: ``
  * Other Info: ``


Instances: 3

### Solution

Ensure that your web server, application server, load balancer, etc. is configured to enforce Strict-Transport-Security.

### Reference


* [ https://cheatsheetseries.owasp.org/cheatsheets/HTTP_Strict_Transport_Security_Cheat_Sheet.html ](https://cheatsheetseries.owasp.org/cheatsheets/HTTP_Strict_Transport_Security_Cheat_Sheet.html)
* [ https://owasp.org/www-community/Security_Headers ](https://owasp.org/www-community/Security_Headers)
* [ https://en.wikipedia.org/wiki/HTTP_Strict_Transport_Security ](https://en.wikipedia.org/wiki/HTTP_Strict_Transport_Security)
* [ https://caniuse.com/stricttransportsecurity ](https://caniuse.com/stricttransportsecurity)
* [ https://datatracker.ietf.org/doc/html/rfc6797 ](https://datatracker.ietf.org/doc/html/rfc6797)


#### CWE Id: [ 319 ](https://cwe.mitre.org/data/definitions/319.html)


#### WASC Id: 15

#### Source ID: 3

### [ Unexpected Content-Type was returned ](https://www.zaproxy.org/docs/alerts/100001/)



##### Low (High)

### Description

A Content-Type of text/html was returned by the server.
This is not one of the types expected to be returned by an API.
Raised by the 'Alert on Unexpected Content Types' script

* URL: https://nginx:8443

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `text/html`
  * Other Info: ``
* URL: https://nginx:8443/

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `text/html`
  * Other Info: ``
* URL: https://nginx:8443/5780466668426731653

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `text/html`
  * Other Info: ``
* URL: https://nginx:8443/api

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `text/html`
  * Other Info: ``
* URL: https://nginx:8443/api/v1

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `text/html`
  * Other Info: ``
* URL: https://nginx:8443/api/v1/

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `text/html`
  * Other Info: ``
* URL: https://nginx:8443/api/v1/actuator/health

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `text/html`
  * Other Info: ``
* URL: https://nginx:8443/api/v1/arquivos

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `text/html`
  * Other Info: ``
* URL: https://nginx:8443/api/v1/arquivos/

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `text/html`
  * Other Info: ``
* URL: https://nginx:8443/api/v1/arquivos/id/

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `text/html`
  * Other Info: ``
* URL: https://nginx:8443/api/v1/arquivos/id/download/

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `text/html`
  * Other Info: ``
* URL: https://nginx:8443/computeMetadata/v1/

  * Method: `POST`
  * Parameter: ``
  * Attack: ``
  * Evidence: `text/html`
  * Other Info: ``
* URL: https://nginx:8443/latest/meta-data/

  * Method: `POST`
  * Parameter: ``
  * Attack: ``
  * Evidence: `text/html`
  * Other Info: ``
* URL: https://nginx:8443/metadata/instance

  * Method: `POST`
  * Parameter: ``
  * Attack: ``
  * Evidence: `text/html`
  * Other Info: ``
* URL: https://nginx:8443/opc/v1/instance/

  * Method: `POST`
  * Parameter: ``
  * Attack: ``
  * Evidence: `text/html`
  * Other Info: ``


Instances: 15

### Solution



### Reference




#### Source ID: 4

### [ ZAP is Out of Date ](https://www.zaproxy.org/docs/alerts/10116/)



##### Low (High)

### Description

The version of ZAP you are using to test your app is out of date and is no longer being updated.
The risk level is set based on how out of date your ZAP version is.

* URL: https://nginx:8443/api/v1/arquivos

  * Method: `POST`
  * Parameter: ``
  * Attack: ``
  * Evidence: ``
  * Other Info: `The latest version of ZAP is 2.17.0`


Instances: 1

### Solution

Download the latest version of ZAP from https://www.zaproxy.org/download/ and install it.

### Reference


* [ https://www.zaproxy.org/download/ ](https://www.zaproxy.org/download/)


#### CWE Id: [ 1104 ](https://cwe.mitre.org/data/definitions/1104.html)


#### WASC Id: 45

#### Source ID: 3

### [ A Client Error response code was returned by the server ](https://www.zaproxy.org/docs/alerts/100000/)



##### Informational (High)

### Description

A response code of 422 was returned by the server.
This may indicate that the application is failing to handle unexpected input correctly.
Raised by the 'Alert on HTTP Response Code Error' script

* URL: https://nginx:8443

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `404`
  * Other Info: ``
* URL: https://nginx:8443/

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `404`
  * Other Info: ``
* URL: https://nginx:8443/5780466668426731653

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `404`
  * Other Info: ``
* URL: https://nginx:8443/api/

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `404`
  * Other Info: ``
* URL: https://nginx:8443/api/4236593326632437177

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `404`
  * Other Info: ``
* URL: https://nginx:8443/api/v1

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `404`
  * Other Info: ``
* URL: https://nginx:8443/api/v1

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `429`
  * Other Info: ``
* URL: https://nginx:8443/api/v1/

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `429`
  * Other Info: ``
* URL: https://nginx:8443/api/v1/5542631705368803878

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `404`
  * Other Info: ``
* URL: https://nginx:8443/api/v1/actuator/health

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `429`
  * Other Info: ``
* URL: https://nginx:8443/api/v1/arquivos

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `404`
  * Other Info: ``
* URL: https://nginx:8443/api/v1/arquivos

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `429`
  * Other Info: ``
* URL: https://nginx:8443/api/v1/arquivos/

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `429`
  * Other Info: ``
* URL: https://nginx:8443/api/v1/arquivos/949641550906203998

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `403`
  * Other Info: ``
* URL: https://nginx:8443/api/v1/arquivos/id

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `403`
  * Other Info: ``
* URL: https://nginx:8443/api/v1/arquivos/id/

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `429`
  * Other Info: ``
* URL: https://nginx:8443/api/v1/arquivos/id/2269648318246456612

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `404`
  * Other Info: ``
* URL: https://nginx:8443/api/v1/arquivos/id/download

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `403`
  * Other Info: ``
* URL: https://nginx:8443/api/v1/arquivos/id/download/

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `429`
  * Other Info: ``
* URL: https://nginx:8443/api/v1/arquivos

  * Method: `POST`
  * Parameter: ``
  * Attack: ``
  * Evidence: `422`
  * Other Info: ``
* URL: https://nginx:8443/api/v1/arquivos/

  * Method: `POST`
  * Parameter: ``
  * Attack: ``
  * Evidence: `422`
  * Other Info: ``
* URL: https://nginx:8443/computeMetadata/v1/

  * Method: `POST`
  * Parameter: ``
  * Attack: ``
  * Evidence: `404`
  * Other Info: ``
* URL: https://nginx:8443/latest/meta-data/

  * Method: `POST`
  * Parameter: ``
  * Attack: ``
  * Evidence: `404`
  * Other Info: ``
* URL: https://nginx:8443/metadata/instance

  * Method: `POST`
  * Parameter: ``
  * Attack: ``
  * Evidence: `404`
  * Other Info: ``
* URL: https://nginx:8443/opc/v1/instance/

  * Method: `POST`
  * Parameter: ``
  * Attack: ``
  * Evidence: `404`
  * Other Info: ``


Instances: 25

### Solution



### Reference



#### CWE Id: [ 388 ](https://cwe.mitre.org/data/definitions/388.html)


#### WASC Id: 20

#### Source ID: 4

### [ Non-Storable Content ](https://www.zaproxy.org/docs/alerts/10049/)



##### Informational (Medium)

### Description

The response contents are not storable by caching components such as proxy servers. If the response does not contain sensitive, personal or user-specific information, it may benefit from being stored and cached, to improve performance.

* URL: https://nginx:8443/api/v1/arquivos/id

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `authorization:`
  * Other Info: ``
* URL: https://nginx:8443/api/v1/arquivos/id/download

  * Method: `GET`
  * Parameter: ``
  * Attack: ``
  * Evidence: `authorization:`
  * Other Info: ``
* URL: https://nginx:8443/api/v1/arquivos

  * Method: `POST`
  * Parameter: ``
  * Attack: ``
  * Evidence: `authorization:`
  * Other Info: ``


Instances: 3

### Solution

The content may be marked as storable by ensuring that the following conditions are satisfied:
The request method must be understood by the cache and defined as being cacheable ("GET", "HEAD", and "POST" are currently defined as cacheable)
The response status code must be understood by the cache (one of the 1XX, 2XX, 3XX, 4XX, or 5XX response classes are generally understood)
The "no-store" cache directive must not appear in the request or response header fields
For caching by "shared" caches such as "proxy" caches, the "private" response directive must not appear in the response
For caching by "shared" caches such as "proxy" caches, the "Authorization" header field must not appear in the request, unless the response explicitly allows it (using one of the "must-revalidate", "public", or "s-maxage" Cache-Control response directives)
In addition to the conditions above, at least one of the following conditions must also be satisfied by the response:
It must contain an "Expires" header field
It must contain a "max-age" response directive
For "shared" caches such as "proxy" caches, it must contain a "s-maxage" response directive
It must contain a "Cache Control Extension" that allows it to be cached
It must have a status code that is defined as cacheable by default (200, 203, 204, 206, 300, 301, 404, 405, 410, 414, 501).

### Reference


* [ https://datatracker.ietf.org/doc/html/rfc7234 ](https://datatracker.ietf.org/doc/html/rfc7234)
* [ https://datatracker.ietf.org/doc/html/rfc7231 ](https://datatracker.ietf.org/doc/html/rfc7231)
* [ https://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html ](https://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html)


#### CWE Id: [ 524 ](https://cwe.mitre.org/data/definitions/524.html)


#### WASC Id: 13

#### Source ID: 3


