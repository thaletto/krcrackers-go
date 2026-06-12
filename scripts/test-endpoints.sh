#!/bin/bash

# Endpoint test script for krcrackers-go
# Tests all API endpoints systematically

set -e

BASE_URL="http://localhost:8080"
COOKIE_JAR="/tmp/test-cookies.txt"
ADMIN_COOKIE_JAR="/tmp/test-admin-cookies.txt"
PASS=0
FAIL=0
TOTAL=0

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Cleanup on exit
cleanup() {
    echo -e "\n${YELLOW}Cleaning up...${NC}"
    # Kill server if we started it
    if [ -n "$SERVER_PID" ]; then
        kill $SERVER_PID 2>/dev/null || true
        wait $SERVER_PID 2>/dev/null || true
    fi
    rm -f "$COOKIE_JAR" "$ADMIN_COOKIE_JAR"
}
trap cleanup EXIT

# Test helper
test_endpoint() {
    local method="$1"
    local endpoint="$2"
    local expected_status="$3"
    local description="$4"
    local data="${5:-}"
    local cookie_file="${6:-}"
    
    TOTAL=$((TOTAL + 1))
    
    local curl_args=(-s -w "\n%{http_code}" -X "$method" "${BASE_URL}${endpoint}")
    
    if [ -n "$data" ]; then
        curl_args+=(-H "Content-Type: application/json" -d "$data")
    fi
    
    if [ -n "$cookie_file" ]; then
        curl_args+=(-b "$cookie_file" -c "$cookie_file")
    fi
    
    local response
    response=$(curl "${curl_args[@]}" 2>/dev/null)
    
    local http_code
    http_code=$(echo "$response" | tail -n1)
    local body
    body=$(echo "$response" | sed '$d')
    
    if [ "$http_code" -eq "$expected_status" ]; then
        echo -e "${GREEN}✓${NC} $method $endpoint - $description (HTTP $http_code)"
        PASS=$((PASS + 1))
    else
        echo -e "${RED}✗${NC} $method $endpoint - $description (expected $expected_status, got $http_code)"
        echo -e "  Response: ${body:0:200}"
        FAIL=$((FAIL + 1))
    fi
    
    echo "$body"
}

# Test multipart endpoint
test_multipart() {
    local endpoint="$1"
    local expected_status="$2"
    local description="$3"
    local cookie_file="$4"
    shift 4
    
    TOTAL=$((TOTAL + 1))
    
    local curl_args=(-s -w "\n%{http_code}" -X POST "${BASE_URL}${endpoint}" -b "$cookie_file" -c "$cookie_file")
    curl_args+=("$@")
    
    local response
    response=$(curl "${curl_args[@]}" 2>/dev/null)
    
    local http_code
    http_code=$(echo "$response" | tail -n1)
    local body
    body=$(echo "$response" | sed '$d')
    
    if [ "$http_code" -eq "$expected_status" ]; then
        echo -e "${GREEN}✓${NC} POST $endpoint - $description (HTTP $http_code)"
        PASS=$((PASS + 1))
    else
        echo -e "${RED}✗${NC} POST $endpoint - $description (expected $expected_status, got $http_code)"
        echo -e "  Response: ${body:0:200}"
        FAIL=$((FAIL + 1))
    fi
    
    echo "$body"
}

echo "========================================"
echo "  krcrackers-go API Test Suite"
echo "========================================"
echo ""

# Kill any existing server processes
echo -e "${YELLOW}Stopping any existing server...${NC}"
pkill -f "go run ." 2>/dev/null || true
pkill -f "krcracker" 2>/dev/null || true
sleep 2
lsof -ti:8080 | xargs kill -9 2>/dev/null || true
sleep 1

# Start server
echo -e "${YELLOW}Starting server...${NC}"
rm -rf .data/
mkdir -p .data
JWT_SECRET="test-secret-key-for-testing-only" nohup go run . > /tmp/test-server.log 2>&1 &
SERVER_PID=$!
sleep 3

# Check server is running
if ! curl -s "$BASE_URL/health" > /dev/null 2>&1; then
    echo -e "${RED}Failed to start server${NC}"
    cat /tmp/test-server.log
    exit 1
fi
echo -e "${GREEN}Server started (PID: $SERVER_PID)${NC}"
echo ""

# Run migrations
echo -e "${YELLOW}Running migrations...${NC}"
JWT_SECRET="test-secret-key-for-testing-only" go run . migrate up 2>&1 | grep -v "^$" || true
echo -e "${GREEN}Migrations complete${NC}"
echo ""

echo "========================================"
echo "  Testing Endpoints"
echo "========================================"
echo ""

# ============================================
# Health
# ============================================
echo -e "${YELLOW}--- Health ---${NC}"
test_endpoint "GET" "/health" 200 "Health check"
echo ""

# ============================================
# Auth
# ============================================
echo -e "${YELLOW}--- Auth ---${NC}"

# Register
test_endpoint "POST" "/auth/register" 201 "Register new user" \
    '{"email":"test@example.com","password":"password123","name":"Test User","phone":"1234567890"}' \
    "$COOKIE_JAR"

# Register duplicate
test_endpoint "POST" "/auth/register" 409 "Register duplicate email" \
    '{"email":"test@example.com","password":"password123","name":"Test User","phone":"1234567890"}' \
    "$COOKIE_JAR"

# Login
test_endpoint "POST" "/auth/login" 200 "Login" \
    '{"email":"test@example.com","password":"password123"}' \
    "$COOKIE_JAR"

# Login wrong password
test_endpoint "POST" "/auth/login" 401 "Login with wrong password" \
    '{"email":"test@example.com","password":"wrongpassword"}' \
    "$COOKIE_JAR"

# Get current user
test_endpoint "GET" "/auth/me" 200 "Get current user" "" "$COOKIE_JAR"

# Get current user without auth
test_endpoint "GET" "/auth/me" 401 "Get current user without auth"

# Google OAuth with invalid token
test_endpoint "POST" "/auth/google" 401 "Google OAuth with invalid token" \
    '{"idToken":"invalid-token"}'

# Refresh token
test_endpoint "POST" "/auth/refresh" 200 "Refresh token" "" "$COOKIE_JAR"

# Logout
test_endpoint "POST" "/auth/logout" 204 "Logout" "" "$COOKIE_JAR"

# Re-login for subsequent tests
test_endpoint "POST" "/auth/login" 200 "Re-login after logout" \
    '{"email":"test@example.com","password":"password123"}' \
    "$COOKIE_JAR"
echo ""

# ============================================
# Create admin user for admin tests
# ============================================
echo -e "${YELLOW}--- Setup Admin ---${NC}"

# Register admin
test_endpoint "POST" "/auth/register" 201 "Register admin user" \
    '{"email":"admin@example.com","password":"admin123","name":"Admin User","phone":"9876543210"}' \
    "$ADMIN_COOKIE_JAR"

# Make admin in DB
sqlite3 .data/dev.sqlite "UPDATE users SET role = 'admin' WHERE email = 'admin@example.com';"

# Logout first to clear cookies, then re-login to get new token with admin role
test_endpoint "POST" "/auth/logout" 204 "Logout admin" "" "$ADMIN_COOKIE_JAR"

# Login as admin (will get new token with admin role)
test_endpoint "POST" "/auth/login" 200 "Login as admin" \
    '{"email":"admin@example.com","password":"admin123"}' \
    "$ADMIN_COOKIE_JAR"
echo ""

# ============================================
# Re-login as regular user for customer tests
# ============================================
test_endpoint "POST" "/auth/login" 200 "Re-login as regular user" \
    '{"email":"test@example.com","password":"password123"}' \
    "$COOKIE_JAR"
echo ""

# ============================================
# Customers
# ============================================
echo -e "${YELLOW}--- Customers ---${NC}"

# Get profile
test_endpoint "GET" "/customers/profile" 200 "Get profile" "" "$COOKIE_JAR"

# Update profile
test_endpoint "PUT" "/customers/profile" 200 "Update profile" \
    '{"name":"Updated Name","phone":"1111111111"}' \
    "$COOKIE_JAR"

# List addresses (empty)
test_endpoint "GET" "/customers/addresses" 200 "List addresses (empty)" "" "$COOKIE_JAR"

# Create address
test_endpoint "POST" "/customers/addresses" 201 "Create address" \
    '{"label":"Home","street":"123 Main St","city":"Mumbai","state":"Maharashtra","pincode":"400001","country":"India","isDefault":true}' \
    "$COOKIE_JAR"

# Create another address
test_endpoint "POST" "/customers/addresses" 201 "Create second address" \
    '{"label":"Work","street":"456 Office Rd","city":"Bangalore","state":"Karnataka","pincode":"560001","country":"India","isDefault":false}' \
    "$COOKIE_JAR"

# List addresses
test_endpoint "GET" "/customers/addresses" 200 "List addresses" "" "$COOKIE_JAR"

# Update address
test_endpoint "PUT" "/customers/addresses/1" 200 "Update address" \
    '{"label":"Home Updated","street":"789 New St","city":"Delhi","state":"Delhi","pincode":"110001","country":"India","isDefault":false}' \
    "$COOKIE_JAR"

# Set default address
test_endpoint "PUT" "/customers/addresses/2/default" 200 "Set default address" "" "$COOKIE_JAR"

# Delete address
test_endpoint "DELETE" "/customers/addresses/1" 204 "Delete address" "" "$COOKIE_JAR"

# Delete non-existent address
test_endpoint "DELETE" "/customers/addresses/999" 404 "Delete non-existent address" "" "$COOKIE_JAR"
echo ""

# ============================================
# Products (public)
# ============================================
echo -e "${YELLOW}--- Products (Public) ---${NC}"

# List products (empty initially)
test_endpoint "GET" "/products" 200 "List products" "" "$COOKIE_JAR"

# Get non-existent product
test_endpoint "GET" "/products/999" 404 "Get non-existent product" "" "$COOKIE_JAR"
echo ""

# ============================================
# Products (Admin)
# ============================================
echo -e "${YELLOW}--- Products (Admin) ---${NC}"

# Create product without auth
test_endpoint "POST" "/admin/products" 401 "Create product without auth" \
    '{"name":"Test Product","price":99.99,"category":"Test"}'

# Create product as admin
test_endpoint "POST" "/admin/products" 201 "Create product as admin" \
    '{"name":"Test Product","price":99.99,"category":"Test","description":"A test product","brand":"TestBrand"}' \
    "$ADMIN_COOKIE_JAR"

# Create another product
test_endpoint "POST" "/admin/products" 201 "Create second product" \
    '{"name":"Another Product","price":149.99,"category":"Test","description":"Another test product"}' \
    "$ADMIN_COOKIE_JAR"

# List products
test_endpoint "GET" "/products" 200 "List products" "" "$COOKIE_JAR"

# Get product
test_endpoint "GET" "/products/1" 200 "Get product" "" "$COOKIE_JAR"

# Search products
test_endpoint "GET" "/products?q=test" 200 "Search products" "" "$COOKIE_JAR"

# Filter products by category
test_endpoint "GET" "/products?category=Test" 200 "Filter by category" "" "$COOKIE_JAR"

# Filter products by price
test_endpoint "GET" "/products?min_price=100&max_price=200" 200 "Filter by price" "" "$COOKIE_JAR"

# Sort products
test_endpoint "GET" "/products?sort=price_asc" 200 "Sort by price ascending" "" "$COOKIE_JAR"

# Update product
test_endpoint "PUT" "/admin/products/1" 200 "Update product" \
    '{"name":"Updated Product","price":199.99,"category":"Updated","description":"Updated description"}' \
    "$ADMIN_COOKIE_JAR"

# Delete product
test_endpoint "DELETE" "/admin/products/2" 204 "Delete product" "" "$ADMIN_COOKIE_JAR"

# Delete non-existent product
test_endpoint "DELETE" "/admin/products/999" 404 "Delete non-existent product" "" "$ADMIN_COOKIE_JAR"
echo ""

# ============================================
# Orders (public)
# ============================================
echo -e "${YELLOW}--- Orders (Public) ---${NC}"

# Create order
test_endpoint "POST" "/orders" 201 "Create order" \
    '{"userName":"Test User","email":"test@example.com","phone":"1111111111","street":"123 Main St","townOrCity":"Mumbai","state":"Maharashtra","pincode":"400001","deliveryRegion":"West","deliveryLocation":"Mumbai","total":99.99,"items":[{"productId":1,"productName":"Updated Product","price":99.99,"quantity":1,"total":99.99}]}' \
    "$COOKIE_JAR"

# List orders
test_endpoint "GET" "/orders" 200 "List orders" "" "$COOKIE_JAR"

# Get order
test_endpoint "GET" "/orders/1" 200 "Get order" "" "$COOKIE_JAR"

# Update order
test_endpoint "PUT" "/orders/1" 200 "Update order" \
    '{"userName":"Updated User","email":"updated@example.com","phone":"2222222222","street":"456 New St","townOrCity":"Delhi","state":"Delhi","pincode":"110001","deliveryRegion":"North","deliveryLocation":"Delhi","total":199.99,"items":[{"productId":1,"productName":"Updated Product","price":199.99,"quantity":1,"total":199.99}]}' \
    "$COOKIE_JAR"

# Get non-existent order
test_endpoint "GET" "/orders/999" 404 "Get non-existent order" "" "$COOKIE_JAR"
echo ""

# ============================================
# Orders (Customer)
# ============================================
echo -e "${YELLOW}--- Orders (Customer) ---${NC}"

# Checkout
test_multipart "/orders/checkout" 201 "Checkout" "$COOKIE_JAR" \
    -F "address_id=2" \
    -F 'items=[{"productId":1,"productName":"Updated Product","price":199.99,"quantity":2}]' \
    -F "payment_reference=PAY123"

# List my orders
test_endpoint "GET" "/orders/my" 200 "List my orders" "" "$COOKIE_JAR"

# Get my order
test_endpoint "GET" "/orders/my/2" 200 "Get my order" "" "$COOKIE_JAR"

# Get someone else's order
test_endpoint "GET" "/orders/my/1" 404 "Get someone else's order" "" "$COOKIE_JAR"

# Cancel my order
test_endpoint "DELETE" "/orders/my/2" 200 "Cancel my order" "" "$COOKIE_JAR"

# Cancel already cancelled order
test_endpoint "DELETE" "/orders/my/2" 422 "Cancel already cancelled order" "" "$COOKIE_JAR"
echo ""

# ============================================
# Orders (Admin)
# ============================================
echo -e "${YELLOW}--- Orders (Admin) ---${NC}"

# List all orders
test_endpoint "GET" "/admin/orders" 200 "List all orders" "" "$ADMIN_COOKIE_JAR"

# Filter orders by status
test_endpoint "GET" "/admin/orders?status=pending" 200 "Filter orders by status" "" "$ADMIN_COOKIE_JAR"

# Get order
test_endpoint "GET" "/admin/orders/1" 200 "Get order" "" "$ADMIN_COOKIE_JAR"

# Update order status (pending -> confirmed)
test_endpoint "PATCH" "/admin/orders/1/status" 200 "Update order status to confirmed" \
    '{"status":"confirmed"}' \
    "$ADMIN_COOKIE_JAR"

# Update order status (confirmed -> shipped)
test_endpoint "PATCH" "/admin/orders/1/status" 200 "Update order status to shipped" \
    '{"status":"shipped"}' \
    "$ADMIN_COOKIE_JAR"

# Update order status (shipped -> delivered)
test_endpoint "PATCH" "/admin/orders/1/status" 200 "Update order status to delivered" \
    '{"status":"delivered"}' \
    "$ADMIN_COOKIE_JAR"

# Invalid status transition
test_endpoint "PATCH" "/admin/orders/1/status" 400 "Invalid status transition" \
    '{"status":"pending"}' \
    "$ADMIN_COOKIE_JAR"

# Get non-existent order
test_endpoint "GET" "/admin/orders/999" 404 "Get non-existent order" "" "$ADMIN_COOKIE_JAR"
echo ""

# ============================================
# Admin Dashboard
# ============================================
echo -e "${YELLOW}--- Admin Dashboard ---${NC}"

# Get dashboard stats
test_endpoint "GET" "/admin/dashboard" 200 "Get dashboard stats" "" "$ADMIN_COOKIE_JAR"

# Get dashboard stats without admin
test_endpoint "GET" "/admin/dashboard" 403 "Get dashboard stats without admin" "" "$COOKIE_JAR"
echo ""

# ============================================
# Invoices
# ============================================
echo -e "${YELLOW}--- Invoices ---${NC}"

# Get invoice (customer)
TOTAL=$((TOTAL + 1))
response=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/invoices/1" -b "$COOKIE_JAR" -o /tmp/invoice.pdf 2>/dev/null)
http_code=$(echo "$response" | tail -n1)
if [ "$http_code" -eq 200 ] && file /tmp/invoice.pdf | grep -q "PDF"; then
    echo -e "${GREEN}✓${NC} GET /invoices/1 - Get invoice PDF (HTTP $http_code)"
    PASS=$((PASS + 1))
else
    echo -e "${RED}✗${NC} GET /invoices/1 - Get invoice PDF (expected 200 with PDF, got $http_code)"
    FAIL=$((FAIL + 1))
fi

# Get invoice (admin)
TOTAL=$((TOTAL + 1))
response=$(curl -s -w "\n%{http_code}" -X GET "${BASE_URL}/admin/invoices/1" -b "$ADMIN_COOKIE_JAR" -o /tmp/admin-invoice.pdf 2>/dev/null)
http_code=$(echo "$response" | tail -n1)
if [ "$http_code" -eq 200 ] && file /tmp/admin-invoice.pdf | grep -q "PDF"; then
    echo -e "${GREEN}✓${NC} GET /admin/invoices/1 - Get invoice PDF as admin (HTTP $http_code)"
    PASS=$((PASS + 1))
else
    echo -e "${RED}✗${NC} GET /admin/invoices/1 - Get invoice PDF as admin (expected 200 with PDF, got $http_code)"
    FAIL=$((FAIL + 1))
fi

# Get non-existent invoice
test_endpoint "GET" "/invoices/999" 404 "Get non-existent invoice" "" "$COOKIE_JAR"

# Get invoice without auth
test_endpoint "GET" "/invoices/1" 401 "Get invoice without auth"
echo ""

# ============================================
# Summary
# ============================================
echo "========================================"
echo "  Test Summary"
echo "========================================"
echo ""
echo -e "Total:  $TOTAL"
echo -e "${GREEN}Passed: $PASS${NC}"
echo -e "${RED}Failed: $FAIL${NC}"
echo ""

if [ $FAIL -eq 0 ]; then
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}Some tests failed.${NC}"
    exit 1
fi
