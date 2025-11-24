#!/bin/bash
# GitHub OAuth Test Script

echo "🔐 Testing GitHub OAuth Flow"
echo "=============================="
echo ""

# First, signup/login to get a token
echo "1️⃣  Creating test user..."
SIGNUP_RESPONSE=$(curl -s -X POST http://localhost:8080/auth/signup \
  -H "Content-Type: application/json" \
  -d '{
    "email": "github-test@example.com",
    "password": "testpassword123"
  }')

TOKEN=$(echo $SIGNUP_RESPONSE | jq -r '.token')

if [ "$TOKEN" = "null" ] || [ -z "$TOKEN" ]; then
  echo "❌ Failed to get auth token. Response: $SIGNUP_RESPONSE"
  exit 1
fi

echo "✅ Got auth token: ${TOKEN:0:20}..."
echo ""

# Check current GitHub status
echo "2️⃣  Checking GitHub link status..."
GITHUB_STATUS=$(curl -s http://localhost:8080/auth/github/status \
  -H "Authorization: Bearer $TOKEN")

echo "Status: $GITHUB_STATUS"
echo ""

# Get GitHub auth URL
echo "3️⃣  Getting GitHub authorization URL..."
echo ""
echo "📋 To link your GitHub account:"
echo "   1. Make sure you're logged in to the app"
echo "   2. Visit this URL in your browser:"
echo ""
echo "      http://localhost:8080/auth/github/login"
echo ""
echo "   3. Authorize the app on GitHub"
echo "   4. Select which repositories to grant access to"
echo ""
echo "💡 After authorization, you can check status again with:"
echo "   curl http://localhost:8080/auth/github/status -H \"Authorization: Bearer $TOKEN\""
