// Copyright (C) 2021, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package constants

// OidcCallbackPath is the callback URL path of OIDC authentication redirect
const OidcCallbackPath = "/_authentication_callback"

// OidcLogoutCallbackPath is the callback URL path after logout
const OidcLogoutCallbackPath = "/_logout"

// OidcConfLua defines the conf.lua file name in OIDC proxy ConfigMap
const OidcConfLua = "conf.lua"

// OidcConfLuaTemp is the template of conf.lua file in OIDC proxy ConfigMap
const OidcConfLuaTemp = `-- conf.lua
    local ingressUri = 'https://'..'%s'
    local oidcProviderHost = "%s"
    local oidcProviderHostInCluster = "%s"
    local realm ="%s"
    local callbackPath = "%s"
    local logoutCallbackPath = "%s"
    local oidcClient = "%s"
    local oidcDirectAccessClient = "%s"
    local cookieKey = "%s"
    local requiredRole = "%s"
    local authStateTtlInSec = %v
    local bearerServiceAccountToken = false

    local oidcProviderInClusterUri = ""
    if oidcProviderHostInCluster and oidcProviderHostInCluster ~= "" then
        oidcProviderInClusterUri = 'http://'..oidcProviderHostInCluster..'/auth/realms/'..realm
    end
    local oidcProviderUri = 'https://'..oidcProviderHost.. '/auth/realms/'..realm
    
    local auth = require("auth").config({
        ingressUri = ingressUri,
        callbackPath = callbackPath,
        callbackUri = ingressUri..callbackPath,
        logoutCallbackUri = ingressUri..logoutCallbackPath,
        oidcProviderUri = oidcProviderUri,
        oidcProviderAuthUri = oidcProviderUri..'/protocol/openid-connect/auth',
        oidcProviderInClusterUri = oidcProviderInClusterUri,
        oidcClient = oidcClient,
        oidcDirectAccessClient = oidcDirectAccessClient,
        authStateTtlInSec = authStateTtlInSec,
        cookieKey = cookieKey,
        bearerServiceAccountToken = bearerServiceAccountToken
    })
    ngx.header["Access-Control-Allow-Origin"] =  ngx.req.get_headers()["origin"]
    ngx.header["Access-Control-Allow-Headers"] =  "authorization"
    if ngx.req.get_method() == "OPTIONS" then
        ngx.status = 200
        ngx.exit(ngx.HTTP_OK)
    end
    auth.info("Extracting authorization header from request.")
    local authHeader = ngx.req.get_headers()["authorization"]
    if not authHeader then
        if string.find(ngx.var.request_uri, callbackPath) then
            auth.handleCallback()
        elseif string.find(ngx.var.request_uri, logoutCallbackPath) then
            auth.handleLogoutCallback()
        end
        local ck = auth.authenticated()
        if ck then
            if auth.hasRequiredRole(ck.it, requiredRole) then
                ngx.var.oidc_user = auth.usernameFromIdToken(ck.it)
            else
                auth.logout()
                return
            end
        else
            auth.authenticate()
        end
    else
        local idToken = auth.authHeader(authHeader)
        if idToken then
            if auth.hasRequiredRole(idToken, requiredRole) then
                ngx.var.oidc_user = auth.usernameFromIdToken(idToken)
            else
                auth.forbidden("User does not have a required realm role")
            end
        end
    end
`

// OidcAuthLua defines the auth.lua file name in OIDC proxy ConfigMap
const OidcAuthLua = "auth.lua"

// OidcAuthLuaScripts is the content of auth.lua file in OIDC proxy ConfigMap
const OidcAuthLuaScripts = `-- auth.lua
    local me = {}
    local random = require("resty.random")
    local base64 = require("ngx.base64")
    local cjson = require "cjson"
    local jwt = require "resty.jwt"
    
    function me.config(opts)
        for key, val in pairs(opts) do
            me[key] = val
        end
        local aes = require "resty.aes"
        me.aes256 = aes:new(me.cookieKey, nil, aes.cipher(256))
        return me
    end
    
    function me.log(logLevel, msg, name, value)
        local logObj = {message = msg}
        if name then
            logObj[name] = value
        end
        ngx.log(logLevel,  cjson.encode(logObj))
    end
    
    function me.logJson(logLevel, msg, err)
        if err then
            me.log(logLevel, msg, 'error', err)
        else
            me.log(logLevel, msg)
        end
    end
      
    function me.info(msg, obj)
        if obj then
            me.log(ngx.INFO, msg, 'object', obj)
        else
            me.log(ngx.INFO, msg)
        end
    end
      
    function me.queryParams(req_uri)
         local i = req_uri:find("?")
         if not i then
             i = 0
         else
             i = i + 1
         end
         return ngx.decode_args(req_uri:sub(i), 0)
    end
      
    function me.query(req_uri, name)
        local i = req_uri:find("&"..name.."=")
        if not i then
        i = req_uri:find("?"..name.."=")
        end
        if not i then
            return nil
        else
            local begin = i+2+name:len()
            local endin = req_uri:find("&", begin)
            if not endin then
                return req_uri:sub(begin)
            end
            return req_uri:sub(begin, endin-1)
        end
    end
      
    function me.unauthorized(msg, err)
        ngx.status = ngx.HTTP_UNAUTHORIZED
        me.logJson(ngx.ERR, msg, err)
        ngx.exit(ngx.HTTP_UNAUTHORIZED)
    end
      
    function me.forbidden(msg, err)
        ngx.status = ngx.HTTP_FORBIDDEN
        me.deleteCookie("authn")
        me.logJson(ngx.ERR, msg, err)
        ngx.exit(ngx.HTTP_FORBIDDEN)
    end

    function me.randomBase64(size)
        local randBytes = random.bytes(size)
        return base64.encode_base64url(randBytes)
    end
      
    function me.read_file(path)
        local file = io.open(path, "rb")
        if not file then return nil end
        local content = file:read "*a"
        file:close()
        return content
    end

    function me.authenticate()
        local sha256 = (require 'resty.sha256'):new()
        local codeVerifier = me.randomBase64(32)
        sha256:update(codeVerifier)
        local codeChallenge = base64.encode_base64url(sha256:final())
        local state = me.randomBase64(32)
        local nonce = me.randomBase64(32)
        local stateData = {
            state = state,
            request_uri = ngx.var.request_uri,
            code_verifier = codeVerifier,
            code_challenge = codeChallenge,
            nonce = nonce
        }
        local redirectArgs = ngx.encode_args({
            client_id = me.oidcClient,
            response_type = 'code',
            scope = 'openid',
            code_challenge_method = 'S256',
            code_challenge = codeChallenge,
            state = state,
            nonce = nonce,
            redirect_uri = me.callbackUri
        })
        local redirtURL = me.oidcProviderAuthUri..'?'..redirectArgs
        me.setCookie("state", stateData, me.authStateTtlInSec)
        ngx.header["Cache-Control"] = "no-cache, no-store, max-age=0"
        ngx.redirect(redirtURL)
    end
      
    function me.tokenRequest(formArgs) 
        local tokenUri = me.oidcProviderUri.."/protocol/openid-connect/token"
        if me.oidcProviderInClusterUri and me.oidcProviderInClusterUri ~= "" then
            tokenUri = me.oidcProviderInClusterUri.."/protocol/openid-connect/token" 
        end
        local http = require "resty.http"
        local httpc = http.new()
        local res, err = httpc:request_uri(tokenUri, {
            method = "POST",
            body = ngx.encode_args(formArgs),
            headers = {
                ["Content-Type"] = "application/x-www-form-urlencoded",
            }
        })
        -- ,keepalive_timeout = 60000,
        -- keepalive_pool = 10
        return cjson.decode(res.body)
    end
      
    function me.authenticated()
        local ck = me.readCookie("authn")
        if ck then
            local rft = ck.rt
            local now = ngx.time()
            local expiry = tonumber(ck.expiry)
            local refresh_expiry = tonumber(ck.refresh_expiry)
            if now > refresh_expiry then
                -- refresh_token expired, redirect to authenticate
                me.authenticate()
            else
                if now > expiry then
                    -- token expired refresh
                    local tokenRef = me.tokenToCookie({
                        grant_type = 'refresh_token',
                        client_id = me.oidcClient,
                        refresh_token = rft,
                        redirect_uri = me.callbackUri
                    })
                    me.info("token refreshed",  tokenRef)
                end
            end
        end
        return ck
    end
      
    function me.handleCallback() 
        local queryParams = me.queryParams(ngx.var.request_uri)
        local state = queryParams.state
        local code = queryParams.code
        local nonce = queryParams.nonce
        local cookie = me.readCookie("state")
        if not cookie then
            me.log(ngx.ERR, "Missing callback state redirect to authenticate")
            me.authenticate()
        end
        local stateCk = cookie.state
        -- local nonceCk = cookie.nonce
        local request_uri = cookie.request_uri

        if (state == nil) or (stateCk == nil) then
            me.log(ngx.ERR, "Missing callback state redirect to authenticate")
            me.authenticate()
        else
            if state ~= stateCk then
                me.log(ngx.ERR, "Invalid callback state redirect to authenticate")
                me.authenticate()
            end
            if not cookie.code_verifier then
                me.log(ngx.ERR, "Invalid callback state redirect to authenticate")
                me.authenticate()
            end
            local formArgs = {
                grant_type = 'authorization_code',
                client_id = me.oidcClient,
                code = code,
                code_verifier = cookie.code_verifier,
                redirect_uri = me.callbackUri
            } 
            local tokenRes = me.tokenToCookie(formArgs)
            ngx.redirect(request_uri) 
        end
    end

    function me.logout()
        local redirectArgs = ngx.encode_args({
            redirect_uri = me.logoutCallbackUri
        })
        local redirURL = me.oidcProviderUri.."/protocol/openid-connect/logout?"..redirectArgs
        ngx.redirect(redirURL)
    end

    function me.handleLogoutCallback()
        me.forbidden("User does not have a required realm role")
    end

    function me.tokenToCookie(formArgs) 
        local tokenRes = me.tokenRequest(formArgs)
        -- Do we need access_token? too big > 4k
        local cookiePairs = {
            rt = tokenRes.refresh_token,
            -- at = tokenRes.access_token,
            it = tokenRes.id_token
        }
        local id_token = jwt:load_jwt(tokenRes.id_token)
        local expires_in = tonumber(tokenRes.expires_in)
        local refresh_expires_in = tonumber(tokenRes.refresh_expires_in)
        local now = ngx.time()
        local issued_at = now
        if id_token and id_token.payload then
            if id_token.payload.iat then
                issued_at = tonumber(id_token.payload.iat)
            else
                if id_token.payload.auth_time then
                    issued_at = tonumber(id_token.payload.auth_time)
                end
            end
            --if id_token.payload.email then
            --    cookiePairs.email = id_token.payload.email
            --end
        end
        local skew = now - issued_at
        -- Expire 30 secs before actual time
        local expiryBuffer = 30 
        cookiePairs.expiry = now + expires_in - skew - expiryBuffer
        cookiePairs.refresh_expiry = now + refresh_expires_in - skew - expiryBuffer
        me.setCookie("authn", cookiePairs, tonumber(tokenRes.refresh_expires_in)-expiryBuffer) 
    end

    function me.hasRequiredRole(idToken, requiredRole)
        local id_token = jwt:load_jwt(idToken)
        if id_token and id_token.payload and id_token.payload.realm_access and id_token.payload.realm_access.roles then
            for _, role in ipairs(id_token.payload.realm_access.roles) do
                if role == requiredRole then
                    return true
                end
            end
        end
        return false
    end

    function me.usernameFromIdToken(idToken)
        local id_token = jwt:load_jwt(idToken)
        if id_token and id_token.payload and id_token.payload.preferred_username then
            return id_token.payload.preferred_username
        end
        return ""
    end

    function me.setCookie(ckName, cookiePairs, expiresInSec) 
        local expires = ngx.cookie_time(ngx.time() + expiresInSec)
        local attributes = "; Path=/; Secure; HttpOnly; Expires="..expires
        local encrypted = me.aes256:encrypt(cjson.encode(cookiePairs))
        local cookie = base64.encode_base64url(encrypted)
        ngx.header["Set-Cookie"] = ckName..'='..cookie..attributes
    end

    function me.deleteCookie(ckName)
        ngx.header["Set-Cookie"] = ckName..'=; Path=/; Secure; HttpOnly; Expires=Thu, 01 Jan 1970 00:00:00 UTC;'
    end

    function me.readCookie(ckName)
        if not ckName then
            return nil
        end
        local cookie, err = require("resty.cookie"):new()
        local ck = cookie:get(ckName)
        if not ck then
            return nil
        end
        local decoded = base64.decode_base64url(ck)
        if not decoded then
            return nil
        end
        local json = me.aes256:decrypt(decoded)
        if not json then    
            return nil
        end
        return cjson.decode(json) 
    end
      
    -- handle auth header: Authorization: <type> <credentials>
    function me.authHeader(authHeader)
        local found, index = authHeader:find('Bearer')
        if found then
            me.info("Extract jwt token from authorization header.")
            local token = string.sub(authHeader, index+2)
            me.handleBearer(token) 
        else
            found, index = authHeader:find('Basic')
            if found then
                local basicCred = string.sub(authHeader, index+2)
                return me.handleBasicAuth(basicCred)
            else
                me.unauthorized("Invalid authorization header "..authHeader)
            end
        end
        return nil
    end

    local certs = {}
    function me.realmCerts(kid) 
        local pk = certs[kid]	
        if pk then
            return pk
        end
        local http = require "resty.http"
        local httpc = http.new()
        local providerUri = me.oidcProviderUri
        if me.oidcProviderInClusterUri and me.oidcProviderInClusterUri ~= "" then
            providerUri = me.oidcProviderInClusterUri
        end
        local certsUri = providerUri..'/protocol/openid-connect/certs'
        local res, err = httpc:request_uri(certsUri)
        if err then
            return nil
        end
        local data = cjson.decode(res.body)
        if not (data.keys) then
            return nil
        end
        for i, key in pairs(data.keys) do
            if key.kid and key.x5c then
                certs[key.kid] = key.x5c
            end
        end
        return certs[kid]
    end

    function me.publicKey(kid) 
        local x5c = me.realmCerts(kid)
        if (not x5c) or (#x5c == 0) then
            return nil
        end
        return "-----BEGIN CERTIFICATE-----\n"..x5c[1].."\n-----END CERTIFICATE-----"
    end
    
    function me.handleBearer(token) 
        if not (token) then
            me.unauthorized("Invalid bearer token in authorization header")    
        end
        me.logJson(ngx.INFO, "Validate JWT token.")
        local jwt = require "resty.jwt"
        local jwt_obj = jwt:load_jwt(token)
        if (not jwt_obj.header) or (not jwt_obj.header.kid) then
            me.unauthorized("Invalid JWT token", jwt_obj.reason)
        end
        local publicKey = me.publicKey(jwt_obj.header.kid)
        if not publicKey then
            me.unauthorized("No public_key retrieved from keycloak")
        end
        local verified = jwt:verify_jwt_obj(publicKey, jwt_obj)
        if (tostring(jwt_obj.valid) == "false" or tostring(jwt_obj.verified) == "false") then
            me.unauthorized("Invalid JWT token", jwt_obj.reason)
        end
        me.logJson(ngx.INFO, "Check for groups in jwt token.")
        if ( not (jwt_obj.payload) or not (jwt_obj.payload.groups)) then
            me.unauthorized("No groups associated with user.")
        end
        ngx.req.clear_header("Authorization")
        if jwt_obj.payload and jwt_obj.payload.email then
            ngx.header['x-proxy-user'] = jwt_obj.payload.email
        end
        if jwt_obj.payload and jwt_obj.payload.k8s_user then
            ngx.header['x-k8s-user'] = jwt_obj.payload.k8s_user
        end
        for i,group in pairs(jwt_obj.payload.groups) do
            ngx.req.set_header("Impersonate-Group", group)
        end
        if me.bearerServiceAccountToken then
            me.logJson(ngx.INFO, "Check for k8s_user in jwt token.")
            if ( not (jwt_obj.payload) or not (jwt_obj.payload.k8s_user)) then
               me.unauthorized("No k8s_user associated with user.")
            end
            me.logJson(ngx.INFO, "Read service account token.")
            local serviceAccountToken = me.read_file("/run/secrets/kubernetes.io/serviceaccount/token");
            if not (serviceAccountToken) then
               me.unauthorized("No service account token presnet in pod.")
            end
            ngx.req.set_header("Authorization", "Bearer " .. serviceAccountToken)
            ngx.req.set_header("Impersonate-User", jwt_obj.payload.k8s_user)
        end
    end
      
    local basicCache = {}
      
    function me.handleBasicAuth(basicCred) 
        local now = ngx.time() 
        local basicAuth = basicCache[basicCred]
        if basicAuth and (now < basicAuth.expiry) then
            return basicAuth.id_token
        end
        local decode = ngx.decode_base64(basicCred)
        local found = decode:find(':')
        if not found then
            me.unauthorized("Invalid BasicAuth authorization header")
        end
        local u = decode:sub(1, found-1)
        local p = decode:sub(found+1)
        local formArgs = {
            grant_type = 'password',
            scope = 'openid',
            client_id = me.oidcDirectAccessClient,
            password = p,
            username = u
        }
        local tokenRes = me.tokenRequest(formArgs)
        if tokenRes.error or tokenRes.error_description then
            me.unauthorized(tokenRes.error_description)
        end
        local refresh_expires_in = tonumber(tokenRes.refresh_expires_in)
        for key, val in pairs(basicCache) do
            if val.expiry and now > val.expiry then
                basicCache[key] = nil
            end
        end
        basicCache[basicCred] = {
            -- access_token = tokenRes.access_token,
            id_token = tokenRes.id_token,
            expiry = now + refresh_expires_in
        }
        return tokenRes.id_token
    end
      
    return me
`

// OidcNginxConf defines the nginx.lua file name in OIDC proxy ConfigMap
const OidcNginxConf = "nginx.conf"

// OidcSslVerifyOptions defines the ssl verify option
const OidcSslVerifyOptions = "lua_ssl_verify_depth 2;"

// OidcSslTrustedOptions defines the ssl trusted certificates option
const OidcSslTrustedOptions = "lua_ssl_trusted_certificate /secret/ca-bundle;"

// OidcNginxConfTemp is the template of nginx.conf file in OIDC proxy ConfigMap
const OidcNginxConfTemp = `#user  nobody;
worker_processes  1;
#error_log  logs/error.log;
#error_log  logs/error.log  notice;
#error_log  logs/error.log  info;
#pid        logs/nginx.pid;
events {
    worker_connections  1024;
}
http {
    include       mime.types;
    default_type  application/octet-stream;

    #log_format  main  '$remote_addr - $remote_user [$time_local] "$request" '
    #                  '$status $body_bytes_sent "$http_referer" '
    #                  '"$http_user_agent" "$http_x_forwarded_for"';
    error_log  logs/error.log  info;
    sendfile        on;
    #tcp_nopush     on;
    client_max_body_size 65m;
    #keepalive_timeout  0;
    keepalive_timeout  65;

    #gzip  on;
    #
    lua_package_path '/usr/local/share/lua/5.1/?.lua;;';
    lua_package_cpath '/usr/local/lib/lua/5.1/?.so;;';
    resolver _NAMESERVER_;
    # cache for discovery metadata documents
    lua_shared_dict discovery 1m;
    #  cache for JWKs
    lua_shared_dict jwks 1m;
    %s
    %s

    upstream http_backend {
        server  %s:%v  fail_timeout=30s max_fails=10;
    }
    server {
        listen       8775;
        server_name  apiserver;
        root     /opt/nginx/html;
        #charset koi8-r;

        set $oidc_user "";
        rewrite_by_lua_file /etc/nginx/conf.lua;
        #access_log  logs/host.access.log  main;
        expires           0;
        add_header        Cache-Control private;
        proxy_set_header  X-WEBAUTH-USER $oidc_user;

        location / {
            proxy_pass http://http_backend;
        }

        error_page 404 /404.html;
            location = /40x.html {
        }

        #error_page  404              /404.html;
        # redirect server error pages to the static page /50x.html
        #
        error_page   500 502 503 504  /50x.html;
        location = /50x.html {
            root   html;
        }
    }
}
`

// OidcStartup defines the startup.sh file name in OIDC proxy ConfigMap
const OidcStartup = "startup.sh"

// OidcStartupTemp is the template of startup.sh file in OIDC proxy ConfigMap
const OidcStartupTemp = `#!/bin/bash
startupDir=%s
cd $startupDir
cp $startupDir/nginx.conf /etc/nginx/nginx.conf
cp $startupDir/auth.lua /etc/nginx/auth.lua
cp $startupDir/conf.lua /etc/nginx/conf.lua
nameserver=$(grep -i nameserver /etc/resolv.conf | awk '{split($0,line," "); print line[2]}')
sed -i -e "s|_NAMESERVER_|${nameserver}|g" /etc/nginx/nginx.conf
mkdir -p /usr/local/nginx/logs
touch /usr/local/nginx/logs/error.log
export LD_LIBRARY_PATH=/usr/local/lib:$LD_LIBRARY_PATH
/usr/local/nginx/sbin/nginx -c /etc/nginx/nginx.conf -t
/usr/local/nginx/sbin/nginx -c /etc/nginx/nginx.conf
while [ $? -ne 0 ]; do
    sleep 20
    echo "retry nginx startup ..."
    /usr/local/nginx/sbin/nginx -c /etc/nginx/nginx.conf
done
tail -f /usr/local/nginx/logs/error.log
`