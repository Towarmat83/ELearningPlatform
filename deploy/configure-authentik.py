#!/usr/bin/env python3
"""Configure Authentik for e-learning platform (OIDC + LDAP + groups + test users).

Env vars:
  REDIRECT_BASE          Frontend base URL for OAuth redirect (default: http://localhost:3000)
  AUTHENTIK_LDAP_IMAGE   goauthentik/ldap image tag (default: ghcr.io/goauthentik/ldap:2026.2.3)
  AUTHENTIK_URL          Authentik base URL to port-forward to (default: http://localhost:9000)
"""

import json, os, subprocess, sys, time
import requests

# ── Configuration ─────────────────────────────────────────────────────────────
# REDIRECT_BASE must match OAUTH_REDIRECT_BASE in kind-values.yaml
REDIRECT_BASE        = os.environ.get("REDIRECT_BASE", "http://localhost:3000").rstrip("/")
REDIRECT_URI         = REDIRECT_BASE + "/auth/callback"
AUTHENTIK_LDAP_IMAGE = os.environ.get("AUTHENTIK_LDAP_IMAGE", "ghcr.io/goauthentik/ldap:2026.2.3")
AUTHENTIK_URL        = os.environ.get("AUTHENTIK_URL", "http://localhost:9000")
# Internal URL used by pods inside the cluster to reach Authentik
AUTHENTIK_INTERNAL   = "http://authentik-server.authentik.svc.cluster.local"

TOKEN = "elearning-bootstrap-token"
BASE  = AUTHENTIK_URL + "/api/v3"

sess = requests.Session()
sess.headers.update({
    "Authorization": f"Bearer {TOKEN}",
    "Content-Type":  "application/json",
    "Accept":        "application/json",
})


def api(method, path, data=None, retry=3):
    url = BASE + path
    for attempt in range(retry):
        try:
            r = sess.request(method, url, json=data, timeout=15)
            if r.status_code in (200, 201):
                try:
                    return r.json()
                except Exception:
                    return {}
            if r.status_code in (204,):
                return {}
            print(f"  !! {method} {path} → HTTP {r.status_code}: {r.text[:200]}", file=sys.stderr)
            return None
        except requests.exceptions.ConnectionError as e:
            if attempt < retry - 1:
                time.sleep(2)
            else:
                print(f"  !! {method} {path} → Connection error: {e}", file=sys.stderr)
                return None


def get(path):       return api("GET",    path)
def post(path, d):   return api("POST",   path, d)
def patch(path, d):  return api("PATCH",  path, d)


def find_one(path, **filters):
    qs = "&".join(f"{k}={v}" for k, v in filters.items())
    r  = get(f"{path}?{qs}&page_size=200")
    if r and r.get("results"):
        return r["results"][0]
    return None


def first_of(path):
    r = get(f"{path}?page_size=200")
    if r and r.get("results"):
        return r["results"][0]
    return None


# ── 1. Groups ──────────────────────────────────────────────────────────────────
print("1. Creating groups…")

def ensure_group(name):
    r = get(f"/core/groups/?name={name}")
    if r and r["results"]:
        pk = r["results"][0]["pk"]
        print(f"   '{name}' exists → {pk}")
        return pk
    r = post("/core/groups/", {"name": name, "is_superuser": False})
    if r:
        print(f"   created '{name}' → {r['pk']}")
        return r["pk"]
    return None

gid_admins   = ensure_group("elearning-admins")
gid_students = ensure_group("elearning-students")

if not gid_admins or not gid_students:
    sys.exit("Failed to create groups")

# ── 2. Test users ──────────────────────────────────────────────────────────────
print("2. Creating test users…")

def ensure_user(username, name, email, password, group_pk):
    r = get(f"/core/users/?username={username}")
    if r and r["results"]:
        uid = r["results"][0]["pk"]
        print(f"   '{username}' exists → {uid}")
    else:
        r = post("/core/users/", {
            "username":  username,
            "name":      name,
            "email":     email,
            "type":      "internal",
            "is_active": True,
            "groups":    [group_pk],
        })
        if not r:
            print(f"   !! failed to create {username}")
            return None
        uid = r["pk"]
        print(f"   created '{username}' → {uid}")

    api("POST", f"/core/users/{uid}/set_password/", {"password": password})

    # Ensure group membership
    user = get(f"/core/users/{uid}/")
    if user:
        groups = user.get("groups", [])
        pks = [g["pk"] if isinstance(g, dict) else g for g in groups]
        if group_pk not in pks:
            patch(f"/core/users/{uid}/", {"groups": pks + [group_pk]})
            print(f"   added to group {group_pk}")
    return uid

ensure_user("test-admin",   "Test Admin",   "testadmin@elearning.local",   "TestAdmin@1234",   gid_admins)
ensure_user("test-student", "Test Student", "teststudent@elearning.local", "TestStudent@1234", gid_students)

# ── 3. Signing certificate ────────────────────────────────────────────────────
print("3. Finding signing certificate…")
cert_pk = None
r = get("/crypto/certificatekeypairs/?has_private_key=true")
if r and r["results"]:
    cert_pk = r["results"][0]["pk"]
    print(f"   using cert → {cert_pk}")
else:
    r = post("/crypto/certificatekeypairs/generate/", {
        "common_name":      "authentik Self-Signed",
        "subject_alt_name": "",
        "validity_days":    3650,
    })
    if r:
        cert_pk = r["pk"]
        print(f"   generated cert → {cert_pk}")

# ── 4. Scope mappings ─────────────────────────────────────────────────────────
print("4. Setting up OIDC scope mappings…")

def get_scope_pk(scope_name):
    r = get(f"/propertymappings/provider/scope/?scope_name={scope_name}")
    if r and r["results"]:
        return r["results"][0]["pk"]
    return None

# Ensure custom groups scope
GROUPS_EXPRESSION = "return [g.name for g in request.user.groups.all()]"
groups_scope_pk = get_scope_pk("groups")
if not groups_scope_pk:
    r = post("/propertymappings/provider/scope/", {
        "name":        "ELearning Groups",
        "scope_name":  "groups",
        "description": "Group membership for role mapping",
        "expression":  GROUPS_EXPRESSION,
    })
    if r:
        groups_scope_pk = r["pk"]
        print(f"   created groups scope → {groups_scope_pk}")
else:
    # Always patch to ensure the expression is up-to-date (ak_groups is deprecated in 2026.x)
    patch(f"/propertymappings/provider/scope/{groups_scope_pk}/", {"expression": GROUPS_EXPRESSION})
    print(f"   groups scope exists, expression patched → {groups_scope_pk}")

scope_openid  = get_scope_pk("openid")
scope_email   = get_scope_pk("email")
scope_profile = get_scope_pk("profile")

all_scopes = [s for s in [scope_openid, scope_email, scope_profile, groups_scope_pk] if s]
print(f"   scopes: {all_scopes}")

# ── 5. Flows ──────────────────────────────────────────────────────────────────
print("5. Finding flows…")

def find_flow(designation, preferred_slugs):
    for slug in preferred_slugs:
        r = find_one("/flows/instances/", slug=slug)
        if r:
            print(f"   {designation}: {r['slug']}")
            return r["pk"]
    r = get(f"/flows/instances/?designation={designation}&page_size=20")
    if r and r["results"]:
        f = r["results"][0]
        print(f"   {designation}: {f['slug']} (fallback)")
        return f["pk"]
    return None

authz_explicit_pk = find_flow("authorization", [
    "default-provider-authorization-explicit-consent",
    "default-authorization-flow",
])
authz_implicit_pk = find_flow("authorization", [
    "default-provider-authorization-implicit-consent",
    "default-authorization-flow",
]) or authz_explicit_pk
authn_pk = find_flow("authentication", [
    "default-authentication-flow",
    "default-source-authentication",
])
invalid_pk = find_flow("invalidation", [
    "default-provider-invalidation-flow",
    "default-invalidation-flow",
])

# ── 6. OIDC Provider ──────────────────────────────────────────────────────────
print(f"6. Creating/updating OIDC provider (redirect_uri={REDIRECT_URI})…")

r = get("/providers/oauth2/?name=ELearning+OIDC")
if r and r["results"]:
    prov = r["results"][0]
    oidc_pk     = prov["pk"]
    client_id   = prov["client_id"]
    client_sec  = prov.get("client_secret", "")
    print(f"   exists → {oidc_pk}")
    # Always patch redirect_uris and property_mappings in case they changed
    patch(f"/providers/oauth2/{oidc_pk}/", {
        "redirect_uris":              [{"matching_mode": "strict", "url": REDIRECT_URI}],
        "property_mappings":          all_scopes,
        "include_claims_in_id_token": True,
        "issuer_mode":                "per_provider",
    })
    print(f"   patched redirect_uri → {REDIRECT_URI}")
    print(f"   patched property_mappings → {all_scopes}")
else:
    payload = {
        "name":                       "ELearning OIDC",
        "authorization_flow":         authz_explicit_pk,
        "client_type":                "confidential",
        "redirect_uris":              [{"matching_mode": "strict", "url": REDIRECT_URI}],
        "property_mappings":          all_scopes,
        "access_token_validity":      "hours=1",
        "refresh_token_validity":     "days=30",
        "include_claims_in_id_token": True,
        "sub_mode":                   "hashed_user_id",
        "issuer_mode":                "per_provider",
    }
    if cert_pk:           payload["signing_key"]        = cert_pk
    if authz_explicit_pk: payload["authorization_flow"] = authz_explicit_pk
    if invalid_pk:        payload["invalidation_flow"]  = invalid_pk

    r = post("/providers/oauth2/", payload)
    if not r:
        sys.exit("Failed to create OIDC provider")
    oidc_pk    = r["pk"]
    client_id  = r["client_id"]
    client_sec = r.get("client_secret", "")
    print(f"   created → {oidc_pk}")
    print(f"   client_id:     {client_id}")
    print(f"   client_secret: {client_sec}")

# Ensure we have the secret (fetch fresh if empty)
if not client_sec:
    r2 = get(f"/providers/oauth2/{oidc_pk}/")
    if r2:
        client_id  = r2.get("client_id", client_id)
        client_sec = r2.get("client_secret", "")

# ── 7. Application (OIDC) ─────────────────────────────────────────────────────
print("7. Creating OIDC application…")
r = get("/core/applications/?slug=elearning-oidc")
if not (r and r["results"]):
    r = post("/core/applications/", {
        "name":             "ELearning Platform",
        "slug":             "elearning-oidc",
        "provider":         oidc_pk,
        "meta_launch_url":  "http://localhost:3000",
    })
    print(f"   created → {r['pk'] if r else 'FAILED'}")
else:
    print(f"   exists → {r['results'][0]['pk']}")
# LDAP provider is assigned as backchannel in step 8b below

# ── 8. LDAP Provider ──────────────────────────────────────────────────────────
print("8. Creating LDAP provider…")
r = get("/providers/ldap/?name=ELearning+LDAP")
if r and r["results"]:
    ldap_pk = r["results"][0]["pk"]
    print(f"   exists → {ldap_pk}")
else:
    payload = {
        "name":     "ELearning LDAP",
        "base_dn":  "dc=ldap,dc=goauthentik,dc=io",
        "search_mode": "direct",
        "bind_mode":   "direct",
        "mfa_support": False,
    }
    if authn_pk:          payload["authentication_flow"]  = authn_pk
    # LDAP direct bind uses authorization_flow for the full bind process (auth+authz)
    # Use the authentication flow so the bind can verify credentials via password stage
    if authn_pk:          payload["authorization_flow"]   = authn_pk
    if invalid_pk:        payload["invalidation_flow"]    = invalid_pk

    r = post("/providers/ldap/", payload)
    if r:
        ldap_pk = r["pk"]
        print(f"   created → {ldap_pk}")
    else:
        ldap_pk = None

# ── 8b. Assign LDAP provider as backchannel to OIDC application ───────────────
if ldap_pk:
    app = get("/core/applications/elearning-oidc/")
    if app:
        existing_bc = app.get("backchannel_providers", [])
        if ldap_pk not in existing_bc:
            patch("/core/applications/elearning-oidc/", {"backchannel_providers": existing_bc + [ldap_pk]})
            print(f"   assigned LDAP provider as backchannel → {ldap_pk}")
        else:
            print(f"   LDAP backchannel already assigned")

# ── 9. Implicit consent flow: add dummy stage so it is not empty ──────────────
print("9. Configuring implicit consent flow…")
implicit_flow = find_one("/flows/instances/", slug="default-provider-authorization-implicit-consent")
if implicit_flow:
    implicit_pk = implicit_flow["pk"]
    # Check if there is already a dummy stage bound
    r = get(f"/flows/bindings/?target={implicit_pk}&page_size=20")
    dummy_exists = any(
        b.get("stage_obj", {}).get("meta_model_name") == "authentik_stages_dummy.dummystage"
        for b in (r or {}).get("results", [])
    )
    if dummy_exists:
        print("   dummy stage already bound")
    else:
        # Create dummy stage
        ds = post("/stages/dummy/", {"name": "ldap-implicit-pass", "throw_error": False})
        if ds:
            binding = post("/flows/bindings/", {
                "target": implicit_pk,
                "stage":  ds["pk"],
                "order":  0,
                "enabled": True,
                "evaluate_on_plan": True,
            })
            print(f"   added dummy stage → {binding['pk'] if binding else 'FAILED'}")
        else:
            print("   failed to create dummy stage")
else:
    print("   implicit consent flow not found")

# ── 10. LDAP Outpost ──────────────────────────────────────────────────────────
print("10. Creating LDAP outpost…")
outpost_pk = None
if ldap_pk:
    r = get("/outposts/instances/?page_size=200")
    existing = next((o for o in (r or {}).get("results", []) if o["name"] == "ELearning LDAP Outpost"), None)
    if not existing:
        r = post("/outposts/instances/", {
            "name":      "ELearning LDAP Outpost",
            "type":      "ldap",
            "providers": [ldap_pk],
            "config":    {"log_level": "info"},
        })
        outpost_pk = r["pk"] if r else None
        print(f"   created → {outpost_pk}")
    else:
        outpost_pk = existing["pk"]
        print(f"   exists → {outpost_pk}")

# ── 11. LDAP Outpost K8s deployment ───────────────────────────────────────────
print("11. Deploying LDAP outpost pod in Kubernetes…")
outpost_token = ""
if outpost_pk:
    outpost_info = get(f"/outposts/instances/{outpost_pk}/")
    token_identifier = outpost_info.get("token_identifier") if outpost_info else None
    if token_identifier:
        view = api("GET", f"/core/tokens/{token_identifier}/view_key/")
        if view:
            outpost_token = view.get("key", "")
            print(f"   token fetched: {outpost_token[:8]}…")
        else:
            print("   !! could not fetch token view_key", file=sys.stderr)
    else:
        print("   !! no token_identifier on outpost", file=sys.stderr)

if outpost_token:
    outpost_yaml = f"""\
apiVersion: apps/v1
kind: Deployment
metadata:
  name: authentik-ldap-outpost
  namespace: authentik
  labels:
    app: authentik-ldap-outpost
spec:
  replicas: 1
  selector:
    matchLabels:
      app: authentik-ldap-outpost
  template:
    metadata:
      labels:
        app: authentik-ldap-outpost
    spec:
      containers:
      - name: ldap
        image: {AUTHENTIK_LDAP_IMAGE}
        env:
        - name: AUTHENTIK_HOST
          value: "{AUTHENTIK_INTERNAL}"
        - name: AUTHENTIK_INSECURE
          value: "true"
        - name: AUTHENTIK_TOKEN
          value: "{outpost_token}"
        ports:
        - containerPort: 3389
          name: ldap
        readinessProbe:
          tcpSocket:
            port: 3389
          initialDelaySeconds: 5
          periodSeconds: 10
---
apiVersion: v1
kind: Service
metadata:
  name: authentik-ldap
  namespace: authentik
spec:
  selector:
    app: authentik-ldap-outpost
  ports:
  - name: ldap
    port: 389
    targetPort: 3389
"""
    result = subprocess.run(
        ["kubectl", "apply", "-f", "-"],
        input=outpost_yaml, text=True, capture_output=True,
    )
    if result.returncode == 0:
        print(f"   {result.stdout.strip()}")
    else:
        print(f"   !! kubectl apply failed:\n{result.stderr.strip()}", file=sys.stderr)
else:
    print("   skipped (no token — run 'kubectl apply' manually)")

# ── Summary ───────────────────────────────────────────────────────────────────
print()
print("=" * 60)
print("CONFIGURATION COMPLETE")
print("=" * 60)
print()
print("OIDC — set these in Admin → Settings:")
print(f"  oidc_enabled:          true")
print(f"  oidc_provider_url:     {AUTHENTIK_INTERNAL}/application/o/elearning-oidc/")
print(f"  oidc_browser_base_url: {AUTHENTIK_URL}  ← rewrites internal URL for browser")
print(f"  oidc_client_id:        {client_id}")
print(f"  oidc_client_secret:    {client_sec}")
print(f"  oidc_scopes:           openid email profile groups")
print(f"  oidc_group_claim:      groups")
print(f"  Redirect URI (Authentik): {REDIRECT_URI}")
print()
print("LDAP — set these in Admin → Settings (after outpost pod is Running):")
print(f"  ldap_enabled:       true")
print(f"  ldap_server_url:    ldap://authentik-ldap.authentik.svc.cluster.local:389")
print(f"  ldap_bind_dn:       cn=akadmin,ou=users,dc=ldap,dc=goauthentik,dc=io")
print(f"  ldap_bind_password: AkAdmin@1234")
print(f"  ldap_user_base_dn:  dc=ldap,dc=goauthentik,dc=io")
print(f"  ldap_user_filter:   (mail=%s)")
print(f"  ldap_group_base_dn: ou=groups,dc=ldap,dc=goauthentik,dc=io")
print()
print("Test users")
print(f"  testadmin@elearning.local   / TestAdmin@1234   → elearning-admins")
print(f"  teststudent@elearning.local / TestStudent@1234 → elearning-students")
print()
print(f"Authentik admin: {AUTHENTIK_URL}  (akadmin / AkAdmin@1234)")
print()
print("Group mappings to configure in e-learning admin:")
print("  elearning-admins   → admin")
print("  elearning-students → student")
