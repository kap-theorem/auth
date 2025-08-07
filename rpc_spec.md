# Authentication as a Service (AaaS) - gRPC Specification v1.0

## Overview

This document defines the gRPC API specification for a centralized Authentication as a Service (AaaS) system. The service provides OAuth 2.0 compliant authentication and session management for multiple independent client applications.

## Architecture Principles

- **Client Isolation**: Each client application has completely isolated user bases
- **Multi-Session Support**: Users can have multiple concurrent sessions across devices
- **OAuth 2.0 Compliance**: Follows industry standard authorization patterns
- **gRPC Protocol**: High-performance binary protocol for internal service communication

## Data Model

### Core Entities

```
Client (1) ──── (N) User (1) ──── (N) Session
```

- **Client**: Independent applications (java-app-1, java-app-2, etc.)
- **User**: End users scoped to specific clients
- **Session**: Active authentication sessions with token pairs

## Service Definition

### Proto File Structure

```protobuf
syntax = "proto3";

package auth.v1;
option go_package = "./proto/auth/v1";
option java_package = "com.example.auth.v1";
option java_outer_classname = "AuthServiceProto";

import "google/protobuf/timestamp.proto";
import "google/protobuf/empty.proto";

service AuthService {
    // Authentication & Session Management
    rpc Login(LoginRequest) returns (LoginResponse);
    rpc RefreshToken(RefreshTokenRequest) returns (RefreshTokenResponse);
    rpc ValidateSession(ValidateSessionRequest) returns (ValidateSessionResponse);
    rpc Logout(LogoutRequest) returns (LogoutResponse);
    rpc LogoutAllSessions(LogoutAllSessionsRequest) returns (LogoutAllSessionsResponse);
    
    // User Management
    rpc RegisterUser(RegisterUserRequest) returns (RegisterUserResponse);
    rpc GetUser(GetUserRequest) returns (GetUserResponse);
    rpc UpdateUser(UpdateUserRequest) returns (UpdateUserResponse);
    rpc ChangePassword(ChangePasswordRequest) returns (ChangePasswordResponse);
    rpc DeactivateUser(DeactivateUserRequest) returns (DeactivateUserResponse);
    
    // Session Management
    rpc GetUserSessions(GetUserSessionsRequest) returns (GetUserSessionsResponse);
    rpc RevokeSession(RevokeSessionRequest) returns (RevokeSessionResponse);
    
    // Client Management (Admin only)
    rpc RegisterClient(RegisterClientRequest) returns (RegisterClientResponse);
    rpc GetClient(GetClientRequest) returns (GetClientResponse);
    rpc UpdateClient(UpdateClientRequest) returns (UpdateClientResponse);
    
    // Health & Monitoring
    rpc HealthCheck(google.protobuf.Empty) returns (HealthCheckResponse);
}
```

## Message Definitions

### Authentication Messages

#### LoginRequest
```protobuf
message LoginRequest {
    string email = 1;               // User email (primary identifier)
    string password = 2;            // User password (plain text, hashed by service)
    string client_id = 3;           // Client application identifier
    string client_secret = 4;       // Client authentication secret
    string user_agent = 5;          // Optional: Browser/device info for session tracking
    int32 session_duration_hours = 6; // Optional: Custom session duration (default: 24h)
}
```

#### LoginResponse
```protobuf
message LoginResponse {
    bool success = 1;
    string access_token = 2;        // JWT access token (30 min default)
    string refresh_token = 3;       // Opaque refresh token (7 days default)
    string session_id = 4;          // Session identifier
    int64 expires_in = 5;           // Access token expiry in seconds
    User user = 6;                  // User information
    AuthError error = 7;            // Error details if success = false
}
```

#### RefreshTokenRequest
```protobuf
message RefreshTokenRequest {
    string refresh_token = 1;       // Current refresh token
    string client_id = 2;           // Client identifier
    string client_secret = 3;       // Client secret
    string user_agent = 4;          // Optional: Updated user agent
}
```

#### RefreshTokenResponse
```protobuf
message RefreshTokenResponse {
    bool success = 1;
    string access_token = 2;        // New JWT access token
    string refresh_token = 3;       // New refresh token (rotation)
    int64 expires_in = 4;           // New access token expiry
    AuthError error = 5;
}
```

#### ValidateSessionRequest
```protobuf
message ValidateSessionRequest {
    string access_token = 1;        // JWT token to validate
    string client_id = 2;           // Requesting client
    string client_secret = 3;       // Client authentication
    bool include_user_details = 4;  // Whether to return full user object
}
```

#### ValidateSessionResponse
```protobuf
message ValidateSessionResponse {
    bool valid = 1;                 // Token validity
    string user_id = 2;             // User identifier
    string session_id = 3;          // Session identifier
    User user = 4;                  // Full user object (if requested)
    repeated string permissions = 5; // User permissions/scopes
    int64 expires_at = 6;           // Token expiry timestamp
    AuthError error = 7;
}
```

#### LogoutRequest
```protobuf
message LogoutRequest {
    string access_token = 1;        // Current access token
    string client_id = 2;
    string client_secret = 3;
    bool revoke_all_sessions = 4;   // Logout from all devices
}
```

#### LogoutResponse
```protobuf
message LogoutResponse {
    bool success = 1;
    string message = 2;
    AuthError error = 3;
}
```

### User Management Messages

#### RegisterUserRequest
```protobuf
message RegisterUserRequest {
    string username = 1;            // Unique within client scope
    string email = 2;               // Unique within client scope
    string password = 3;            // Plain text password
    string client_id = 4;
    string client_secret = 5;
    map<string, string> metadata = 6; // Custom user attributes
}
```

#### RegisterUserResponse
```protobuf
message RegisterUserResponse {
    bool success = 1;
    User user = 2;                  // Created user (without password)
    AuthError error = 3;
}
```

#### GetUserRequest
```protobuf
message GetUserRequest {
    string user_id = 1;             // Target user ID
    string client_id = 2;           // Requesting client
    string client_secret = 3;
    string requesting_access_token = 4; // Optional: For user context
}
```

#### GetUserResponse
```protobuf
message GetUserResponse {
    bool success = 1;
    User user = 2;
    AuthError error = 3;
}
```

#### UpdateUserRequest
```protobuf
message UpdateUserRequest {
    string user_id = 1;
    string client_id = 2;
    string client_secret = 3;
    string requesting_access_token = 4; // Authorization context
    
    // Optional fields to update
    optional string username = 5;
    optional string email = 6;
    map<string, string> metadata = 7;
}
```

#### ChangePasswordRequest
```protobuf
message ChangePasswordRequest {
    string user_id = 1;
    string current_password = 2;
    string new_password = 3;
    string client_id = 4;
    string client_secret = 5;
    bool invalidate_other_sessions = 6; // Force re-login on other devices
}
```

### Session Management Messages

#### GetUserSessionsRequest
```protobuf
message GetUserSessionsRequest {
    string user_id = 1;
    string client_id = 2;
    string client_secret = 3;
    string requesting_access_token = 4; // Must be user's own token
    bool include_expired = 5;       // Include expired sessions
}
```

#### GetUserSessionsResponse
```protobuf
message GetUserSessionsResponse {
    bool success = 1;
    repeated Session sessions = 2;
    AuthError error = 3;
}
```

#### RevokeSessionRequest
```protobuf
message RevokeSessionRequest {
    string session_id = 1;
    string client_id = 2;
    string client_secret = 3;
    string requesting_access_token = 4; // Authorization
}
```

### Client Management Messages

#### RegisterClientRequest
```protobuf
message RegisterClientRequest {
    string client_id = 1;           // Desired client identifier
    string client_name = 2;         // Human readable name
    string admin_secret = 3;        // Admin authentication for client registration
}
```

#### RegisterClientResponse
```protobuf
message RegisterClientResponse {
    bool success = 1;
    string client_id = 2;
    string client_secret = 3;       // Generated secret
    AuthError error = 4;
}
```

## Data Transfer Objects

### User Object
```protobuf
message User {
    string user_id = 1;
    string username = 2;
    string email = 3;
    string client_id = 4;
    google.protobuf.Timestamp created_at = 5;
    google.protobuf.Timestamp updated_at = 6;
    bool active = 7;
    map<string, string> metadata = 8;
    // Note: password field never included in responses
}
```

### Session Object
```protobuf
message Session {
    string session_id = 1;
    string user_id = 2;
    string user_agent = 3;
    google.protobuf.Timestamp created_at = 4;
    google.protobuf.Timestamp expires_at = 5;
    bool active = 6;
    google.protobuf.Timestamp last_used = 7;
    // Note: tokens not included for security
}
```

### Client Object
```protobuf
message Client {
    string client_id = 1;
    string client_name = 2;
    google.protobuf.Timestamp created_at = 3;
    bool active = 4;
    // Note: client_secret never included in responses
}
```

### Error Handling
```protobuf
message AuthError {
    ErrorCode code = 1;
    string message = 2;
    map<string, string> details = 3;
}

enum ErrorCode {
    UNKNOWN = 0;
    INVALID_CREDENTIALS = 1;
    INVALID_CLIENT = 2;
    INVALID_TOKEN = 3;
    TOKEN_EXPIRED = 4;
    USER_NOT_FOUND = 5;
    USER_ALREADY_EXISTS = 6;
    SESSION_NOT_FOUND = 7;
    INSUFFICIENT_PERMISSIONS = 8;
    VALIDATION_ERROR = 9;
    INTERNAL_ERROR = 10;
    RATE_LIMIT_EXCEEDED = 11;
}
```

### Health Check
```protobuf
message HealthCheckResponse {
    enum Status {
        SERVING = 0;
        NOT_SERVING = 1;
        SERVICE_UNKNOWN = 2;
    }
    Status status = 1;
    string message = 2;
    map<string, string> details = 3; // DB status, Redis status, etc.
}
```

## Security Specifications

### Client Authentication
- **Method**: Client credentials (client_id + client_secret)
- **Secret Storage**: Bcrypt hashed with cost 12
- **Validation**: Constant-time comparison to prevent timing attacks
- **Rotation**: Support for client secret rotation

### Token Specifications

#### Access Token (JWT)
```json
{
  "sub": "user_id",           // Subject: User ID
  "aud": "client_id",         // Audience: Client ID
  "iss": "auth-service",      // Issuer: Your service
  "exp": 1234567890,          // Expiry timestamp
  "iat": 1234567890,          // Issued at timestamp
  "session_id": "sess_123",   // Session identifier
  "client_id": "java-app-1"   // Issued to client
}
```

#### Refresh Token
- **Format**: Opaque string (UUID v4)
- **Storage**: Hashed in database
- **Rotation**: New refresh token on each use
- **Revocation**: Support for immediate revocation

### Password Security
- **Hashing**: Bcrypt with cost 12 minimum
- **Policy**: Minimum 8 characters (configurable per client)
- **Validation**: Server-side validation only

## Client Integration Patterns

### Java Client Implementation
```java
@Service
public class AuthServiceClient {
    
    private final AuthServiceGrpc.AuthServiceBlockingStub authStub;
    
    @Value("${auth.client.id}")
    private String clientId;
    
    @Value("${auth.client.secret}")  
    private String clientSecret;
    
    public LoginResponse login(String email, String password, String userAgent) {
        LoginRequest request = LoginRequest.newBuilder()
            .setEmail(email)
            .setPassword(password)
            .setClientId(clientId)
            .setClientSecret(clientSecret)
            .setUserAgent(userAgent)
            .build();
            
        return authStub.login(request);
    }
    
    public ValidateSessionResponse validateSession(String accessToken) {
        ValidateSessionRequest request = ValidateSessionRequest.newBuilder()
            .setAccessToken(accessToken)
            .setClientId(clientId)
            .setClientSecret(clientSecret)
            .setIncludeUserDetails(true)
            .build();
            
        return authStub.validateSession(request);
    }
}
```

## Error Handling Specification

### Standard Error Responses
All RPC methods return errors in the `AuthError` message field, never as gRPC status errors.

#### Error Code Mapping
- `INVALID_CREDENTIALS`: Wrong email/password combination
- `INVALID_CLIENT`: Wrong client_id/client_secret combination  
- `INVALID_TOKEN`: Malformed or invalid access/refresh token
- `TOKEN_EXPIRED`: Valid token but expired
- `USER_NOT_FOUND`: User doesn't exist in this client scope
- `USER_ALREADY_EXISTS`: Email/username already taken in client scope
- `SESSION_NOT_FOUND`: Session doesn't exist or expired
- `INSUFFICIENT_PERMISSIONS`: Client not authorized for operation
- `VALIDATION_ERROR`: Input validation failed
- `RATE_LIMIT_EXCEEDED`: Too many requests from client
- `INTERNAL_ERROR`: Server-side error

### Client Error Handling Pattern
```java
LoginResponse response = authClient.login(email, password, userAgent);
if (!response.getSuccess()) {
    switch (response.getError().getCode()) {
        case INVALID_CREDENTIALS:
            throw new BadCredentialsException("Invalid login");
        case INVALID_CLIENT:
            throw new ClientAuthenticationException("Client auth failed");
        case RATE_LIMIT_EXCEEDED:
            throw new TooManyRequestsException("Rate limited");
        default:
            throw new AuthServiceException("Auth service error");
    }
}
```

## Security Requirements

### Transport Security
- **Production**: TLS 1.3 for all gRPC communications
- **Development**: Plain text acceptable for local development
- **Certificates**: Valid SSL certificates for production deployment

### Authentication Flow Security
1. **Client Validation**: Every RPC call must include valid client credentials
2. **User Scoping**: Users are always scoped to the requesting client
3. **Token Binding**: Tokens are bound to specific client and cannot be cross-used
4. **Session Isolation**: Sessions are isolated per client application

### Rate Limiting
- **Login Attempts**: 5 attempts per email per client per 15 minutes
- **Token Validation**: 1000 requests per client per minute
- **User Registration**: 10 registrations per client per hour
- **Implementation**: Token bucket algorithm with Redis backing

## Implementation Guidelines

### Database Constraints
```sql
-- Ensure email uniqueness within client scope
UNIQUE(email, client_id)

-- Ensure username uniqueness within client scope  
UNIQUE(username, client_id)

-- Cascade delete sessions when user is deleted
FOREIGN KEY (user_id) REFERENCES users(user_id) ON DELETE CASCADE
```

### JWT Token Configuration
- **Algorithm**: RS256 (asymmetric signing)
- **Access Token Lifetime**: 30 minutes (configurable)
- **Refresh Token Lifetime**: 7 days (configurable)
- **Key Rotation**: Support for multiple signing keys

### Session Management
- **Concurrent Sessions**: Unlimited per user (configurable)
- **Session Cleanup**: Automated cleanup of expired sessions
- **Session Tracking**: Track last used timestamp for analytics

## Deployment Specification

### Service Configuration
```yaml
server:
  port: 9090
  tls:
    enabled: true
    cert_file: "/certs/server.crt"
    key_file: "/certs/server.key"

database:
  host: "postgres"
  port: 5432
  database: "auth"
  username: "auth_user"
  password: "${DB_PASSWORD}"
  
redis:
  host: "redis"
  port: 6379
  password: "${REDIS_PASSWORD}"

jwt:
  private_key_file: "/keys/jwt-private.pem"
  public_key_file: "/keys/jwt-public.pem"
  access_token_duration: "30m"
  refresh_token_duration: "168h" # 7 days

rate_limiting:
  login_attempts: 5
  login_window: "15m"
  token_validation_limit: 1000
  token_validation_window: "1m"
```

### Docker Deployment
```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o auth-service cmd/server/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /root/
COPY --from=builder /app/auth-service .
EXPOSE 9090
CMD ["./auth-service"]
```

## Testing Specification

### Unit Test Coverage
- **Client Authentication**: Valid/invalid client credentials
- **User Authentication**: Valid/invalid user credentials  
- **Token Validation**: Valid/expired/malformed tokens
- **Session Management**: Creation, validation, revocation
- **Cross-Client Isolation**: Ensure no data leakage between clients

### Integration Test Scenarios
1. **Complete Login Flow**: Register → Login → Validate → Refresh → Logout
2. **Multi-Session**: Multiple concurrent sessions per user
3. **Client Isolation**: Client A cannot access Client B's users
4. **Token Security**: Tokens issued to Client A rejected by Client B
5. **Error Handling**: All error conditions return proper error codes

### Performance Benchmarks
- **Login**: < 100ms p95
- **Token Validation**: < 10ms p95  
- **Token Refresh**: < 50ms p95
- **Concurrent Users**: Support 1000+ concurrent sessions
- **Throughput**: 10,000+ token validations per second

## Monitoring & Observability

### Metrics to Collect
- Login success/failure rates per client
- Token validation request rates
- Session duration analytics  
- Error rate distribution
- Response time percentiles

### Logging Requirements
```json
{
  "timestamp": "2025-08-03T12:00:00Z",
  "level": "INFO",
  "event": "user_login",
  "client_id": "java-app-1",
  "user_id": "user_123",
  "session_id": "sess_456", 
  "user_agent": "Chrome/91.0",
  "ip_address": "192.168.1.100",
  "success": true
}
```

## API Versioning

### Version Strategy
- **Proto Package**: `auth.v1`, `auth.v2`, etc.
- **Backward Compatibility**: Maintain v1 for 12 months after v2 release
- **Migration Path**: Gradual client migration with parallel version support

### Breaking Changes
Breaking changes require new major version:
- Removing RPC methods
- Removing message fields
- Changing field types
- Changing error semantics

## Client SDK Requirements

### Java SDK Features
- Connection pooling and keep-alive
- Automatic token refresh
- Circuit breaker for fault tolerance
- Caching for token validation results
- Spring Security integration
- Async/reactive support

### SDK Configuration
```yaml
auth:
  service:
    host: auth-service.internal
    port: 9090
    tls: true
  client:
    id: ${AUTH_CLIENT_ID}
    secret: ${AUTH_CLIENT_SECRET}
  cache:
    token_validation_ttl: 300 # 5 minutes
  retry:
    max_attempts: 3
    backoff_multiplier: 2
```

## Migration & Rollout Strategy

### Phase 1: Core Authentication
- Implement Login, ValidateSession, Logout
- Basic user management
- Single client support

### Phase 2: Multi-Client Support  
- Client registration and isolation
- Enhanced security validation
- Rate limiting

### Phase 3: Advanced Features
- Session management APIs
- User profile management
- Monitoring and metrics

### Phase 4: Production Hardening
- TLS implementation
- Performance optimization
- Security audit and penetration testing

---

**Version**: 1.0  
**Last Updated**: August 3, 2025  
**Status**: Draft for Implementation