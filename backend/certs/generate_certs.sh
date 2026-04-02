#!/bin/bash
# TLS Certificate Generation Script for Enterprise RAT
# This script generates CA, Server, and Agent certificates for mTLS authentication
#
# Usage:
#   ./generate_certs.sh                    # Uses localhost (development)
#   ./generate_certs.sh 192.168.1.100      # Uses IP address
#   ./generate_certs.sh myrat.example.com   # Uses domain name
#   ./generate_certs.sh --regenerate        # Regenerate with current domain

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CERTS_DIR="${SCRIPT_DIR}/certs"

mkdir -p "${CERTS_DIR}"

echo "=============================================="
echo " Enterprise RAT - TLS Certificate Generator "
echo "=============================================="
echo ""

# Handle --regenerate flag
REGENERATE=false
if [ "$1" == "--regenerate" ]; then
    REGENERATE=true
    if [ -n "$2" ]; then
        DOMAIN="$2"
    elif [ -f "${CERTS_DIR}/.last_domain" ]; then
        DOMAIN=$(cat "${CERTS_DIR}/.last_domain")
    else
        DOMAIN="localhost"
    fi
elif [ -n "$1" ]; then
    DOMAIN="$1"
else
    DOMAIN="localhost"
fi

echo "Target domain/IP: ${DOMAIN}"
echo "Certificate directory: ${CERTS_DIR}"
echo ""

# Save domain for regeneration
echo -n "${DOMAIN}" > "${CERTS_DIR}/.last_domain"

# Check if DOMAIN is an IP address
is_ip() {
    local ip="$1"
    if [[ "$ip" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        return 0
    fi
    return 1
}

# Configuration
CA_COUNTRY="US"
CA_STATE="California"
CA_LOCALITY="San Francisco"
CA_ORG="Enterprise RAT"
CA_OU="Security"
CA_DAYS=3650

SERVER_COUNTRY="${CA_COUNTRY}"
SERVER_STATE="${CA_STATE}"
SERVER_LOCALITY="${CA_LOCALITY}"
SERVER_ORG="${CA_ORG}"
SERVER_OU="Server"
SERVER_DAYS=365

AGENT_COUNTRY="${CA_COUNTRY}"
AGENT_STATE="${CA_STATE}"
AGENT_LOCALITY="${CA_LOCALITY}"
AGENT_ORG="${CA_ORG}"
AGENT_OU="Agent"
AGENT_DAYS=365

# Cleanup old certificates
cleanup() {
    echo "Cleaning up old certificates..."
    rm -rf "${CERTS_DIR}"/*.pem
    rm -rf "${CERTS_DIR}"/*.key
    rm -rf "${CERTS_DIR}"/*.crt
    rm -rf "${CERTS_DIR}"/*.csr
    rm -rf "${CERTS_DIR}"/*.ext
}

if [ "$REGENERATE" = true ]; then
    cleanup
fi

echo "Step 1: Generating CA private key..."
openssl genrsa -out "${CERTS_DIR}/ca.key" 4096 2>/dev/null
chmod 600 "${CERTS_DIR}/ca.key"

echo "Step 2: Generating CA certificate..."
openssl req -new -x509 \
    -key "${CERTS_DIR}/ca.key" \
    -out "${CERTS_DIR}/ca.crt" \
    -days "${CA_DAYS}" \
    -subj "/C=${CA_COUNTRY}/ST=${CA_STATE}/L=${CA_LOCALITY}/O=${CA_ORG}/OU=${CA_OU}/CN=Enterprise RAT CA" \
    2>/dev/null

echo "Step 3: Generating Server private key..."
openssl genrsa -out "${CERTS_DIR}/server.key" 4096 2>/dev/null
chmod 600 "${CERTS_DIR}/server.key"

echo "Step 4: Generating Server CSR..."

# Build SAN entries dynamically based on DOMAIN
if is_ip "${DOMAIN}"; then
    CN="${DOMAIN}"
    SAN_BLOCK="subjectAltName = @alt_names
extendedKeyUsage = serverAuth, clientAuth

[alt_names]
IP.1 = ${DOMAIN}
DNS.1 = localhost
DNS.2 = *.localhost
IP.2 = 127.0.0.1
IP.3 = ::1"
else
    CN="${DOMAIN}"
    SAN_BLOCK="subjectAltName = @alt_names
extendedKeyUsage = serverAuth, clientAuth

[alt_names]
DNS.1 = ${DOMAIN}
DNS.2 = *.${DOMAIN}
DNS.3 = localhost
DNS.4 = *.localhost
IP.1 = 127.0.0.1
IP.2 = ::1"
fi

openssl req -new \
    -key "${CERTS_DIR}/server.key" \
    -out "${CERTS_DIR}/server.csr" \
    -subj "/C=${SERVER_COUNTRY}/ST=${SERVER_STATE}/L=${SERVER_LOCALITY}/O=${SERVER_ORG}/OU=${SERVER_OU}/CN=${CN}" \
    2>/dev/null

echo "Step 5: Creating Server certificate extensions..."
echo "${SAN_BLOCK}" > "${CERTS_DIR}/server.ext"

echo "Step 6: Signing Server certificate with CA..."
openssl x509 -req \
    -in "${CERTS_DIR}/server.csr" \
    -CA "${CERTS_DIR}/ca.crt" \
    -CAkey "${CERTS_DIR}/ca.key" \
    -out "${CERTS_DIR}/server.crt" \
    -days "${SERVER_DAYS}" \
    -CAcreateserial \
    -extfile "${CERTS_DIR}/server.ext" \
    2>/dev/null

echo "Step 7: Generating Agent private key..."
openssl genrsa -out "${CERTS_DIR}/agent.key" 4096 2>/dev/null
chmod 600 "${CERTS_DIR}/agent.key"

echo "Step 8: Generating Agent CSR..."
openssl req -new \
    -key "${CERTS_DIR}/agent.key" \
    -out "${CERTS_DIR}/agent.csr" \
    -subj "/C=${AGENT_COUNTRY}/ST=${AGENT_STATE}/L=${AGENT_LOCALITY}/O=${AGENT_ORG}/OU=${AGENT_OU}/CN=agent" \
    2>/dev/null

echo "Step 9: Creating Agent certificate extensions..."
cat > "${CERTS_DIR}/agent.ext" << 'EOF'
extendedKeyUsage = clientAuth, serverAuth
basicConstraints = CA:FALSE
keyUsage = digitalSignature, keyEncipherment
EOF

echo "Step 10: Signing Agent certificate with CA..."
openssl x509 -req \
    -in "${CERTS_DIR}/agent.csr" \
    -CA "${CERTS_DIR}/ca.crt" \
    -CAkey "${CERTS_DIR}/ca.key" \
    -out "${CERTS_DIR}/agent.crt" \
    -days "${AGENT_DAYS}" \
    -CAcreateserial \
    -extfile "${CERTS_DIR}/agent.ext" \
    2>/dev/null

echo "Step 11: Creating Agent certificate bundle..."
cat "${CERTS_DIR}/agent.crt" "${CERTS_DIR}/ca.crt" > "${CERTS_DIR}/agent.bundle.crt"

echo "Step 12: Verifying certificates..."
echo ""

echo "=== CA Certificate ==="
openssl x509 -in "${CERTS_DIR}/ca.crt" -noout -subject -dates

echo ""
echo "=== Server Certificate ==="
openssl x509 -in "${CERTS_DIR}/server.crt" -noout -subject -dates

echo ""
echo "=== Agent Certificate ==="
openssl x509 -in "${CERTS_DIR}/agent.crt" -noout -subject -dates

echo ""
echo "=== Certificate Chain Verification ==="
openssl verify -CAfile "${CERTS_DIR}/ca.crt" "${CERTS_DIR}/server.crt"
openssl verify -CAfile "${CERTS_DIR}/ca.crt" "${CERTS_DIR}/agent.crt"

echo ""
echo "Step 13: Setting permissions..."
chmod 400 "${CERTS_DIR}"/*.crt
chmod 400 "${CERTS_DIR}"/*.key
chmod 644 "${CERTS_DIR}"/*.pem
chmod 644 "${CERTS_DIR}"/*.ext
chmod 644 "${CERTS_DIR}"/*.csr
chmod 644 "${CERTS_DIR}"/*.srl
chmod 644 "${CERTS_DIR}"/.last_domain

echo ""
echo "Step 14: Creating README..."
cat > "${CERTS_DIR}/README.md" << EOF
# Enterprise RAT TLS Certificates

This directory contains the mTLS certificates for secure communication.

Generated for: ${DOMAIN}

## Files

- \`ca.crt\` - Root CA certificate (trust this on clients/servers)
- \`ca.key\` - Root CA private key (keep secret!)
- \`server.crt\` - Server certificate signed by CA
- \`server.key\` - Server private key (keep secret!)
- \`agent.crt\` - Agent certificate signed by CA
- \`agent.key\` - Agent private key (keep secret!)
- \`agent.bundle.crt\` - Agent cert + CA cert (for agent use)

## Usage

### Backend Server
\`\`\`go
cert, _ := tls.LoadX509KeyPair("certs/server.crt", "certs/server.key")
caCert, _ := os.ReadFile("certs/ca.crt")
caCertPool := x509.NewCertPool()
caCertPool.AppendCertsFromPEM(caCert)

config := &tls.Config{
    Certificates: []tls.Certificate{cert},
    ClientCAs:    caCertPool,
    ClientAuth:    tls.RequireAndVerifyClientCert,
}
\`\`\`

### Agent
\`\`\`go
cert, _ := tls.LoadX509KeyPair("certs/agent.bundle.crt", "certs/agent.key")
caCert, _ := os.ReadFile("certs/ca.crt")
caCertPool := x509.NewCertPool()
caCertPool.AppendCertsFromPEM(caCert)

config := &tls.Config{
    Certificates: []tls.Certificate{cert},
    RootCAs:      caCertPool,
}
\`\`\`

## Security

- Never commit private keys to version control
- Store keys securely (use Vault, AWS KMS, etc.)
- Rotate certificates regularly
- Use hardware security modules for production

EOF

echo ""
echo "=============================================="
echo " Certificate generation complete! "
echo "=============================================="
echo ""
echo "Target: ${DOMAIN}"
echo "Files created in: ${CERTS_DIR}"
echo ""
echo "To regenerate certificates, run:"
echo "  ./generate_certs.sh --regenerate ${DOMAIN}"
echo "  or"
echo "  ./generate_certs.sh --regenerate (uses last domain)"
echo ""