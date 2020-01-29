# Okta JWT Verifier for Golang

This library helps you verify tokens that have been issued by Okta. To learn more about verification cases and Okta's tokens please read [Working With OAuth 2.0 Tokens](https://developer.okta.com/authentication-guide/tokens/)

## :warning: :construction: Beta Preview :construction: :warning:

This library is under development and is currently in 0.x version series.  Breaking changes may be introduced at minor versions in the 0.x range.  Please lock your dependency to a specific version until this library reaches 1.x.

Need help? [Okta Developer Forum].

## Installation
```sh
go get -u github.com/okta/okta-jwt-verifier-golang
```

## Usage

This library was built to keep configuration to a minimum. To get it running at its most basic form, all you need to provide is the the following information:

- **Issuer** - This is the URL of the authorization server that will perform authentication.  All Developer Accounts have a "default" authorization server.  The issuer is a combination of your Org URL (found in the upper right of the console home page) and `/oauth2/default`. For example, `https://dev-1234.oktapreview.com/oauth2/default`.
- **Client ID**- These can be found on the "General" tab of the Web application that you created earlier in the Okta Developer Console.

#### Access Token Validation
```go
import github.com/okta/okta-jwt-verifier-golang

toValidate := map[string]string{}
toValidate["aud"] = "api://default"
toValidate["cid"] = "{CLIENT_ID}"

jwtVerifierSetup := jwtverifier.JwtVerifier{
        Issuer: "{ISSUER}",
        ClaimsToValidate: toValidate
}

verifier := jwtVerifierSetup.New()

token, err := verifier.VerifyAccessToken("{JWT}")
```

#### Id Token Validation
```go
import github.com/okta/okta-jwt-verifier-golang

toValidate := map[string]string{}
toValidate["nonce"] = "{NONCE}"
toValidate["aud"] = "{CLIENT_ID}"


jwtVerifierSetup := jwtverifier.JwtVerifier{
        Issuer: "{ISSUER}",
        ClaimsToValidate: toValidate
}

verifier := jwtVerifierSetup.New()

token, err := verifier.VerifyIdToken("{JWT}")
```

This will either provide you with the token which gives you access to all the claims, or an error. The token struct contains a `Claims` property that will give you a `map[string]interface{}` of all the claims in the token.

```go
// Getting the sub from the token
sub := token.Claims["sub"]
```

#### Dealing with clock skew
We default to a PT2M clock skew adjustment in our validation.  If you need to change this, you can use the `SetLeeway` method:

```go
jwtVerifierSetup := JwtVerifier{
        Issuer: "{ISSUER}",
}

verifier := jwtVerifierSetup.New()
verifier.SetLeeway(60) // seconds
```

[Okta Developer Forum]: https://devforum.okta.com/
