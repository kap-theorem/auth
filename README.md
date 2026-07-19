# Auth Service

A comprehensive Authentication as a Service (AaaS) built with Go and gRPC. This service provides user authentication, session management, and client registration capabilities.

## Features

- **User Management**: Registration, login, and profile management
- **JWT Authentication**: Secure token-based authentication
- **Session Management**: Refresh tokens and session handling
- **Client Management**: Multi-client support with client registration
- **Security**: Password hashing, token validation, and session expiry
- **Database**: MySQL with GORM ORM
- **Health Checks**: Built-in health monitoring
- **Cleanup Service**: Automatic expired session cleanup

## Architecture

```
├── cmd/server/          # Application entry point
├── internal/database/   # Database connection and setup
├── pkg/
│   ├── models/         # Data models
│   ├── repository/     # Data access layer
│   ├── service/        # Business logic
│   └── utils/          # Utility functions
├── proto/auth/v1/      # Protocol buffer definitions
└── .env.example        # Environment variables template
```

## Prerequisites

- Go 1.21+
- MySQL 8.0+
- Protocol Buffers compiler (protoc)
- gRPC tools

## Installation

1. Clone the repository:
```bash
git clone <repository-url>
cd auth
```

2. Install dependencies:
```bash
go mod tidy
```

3. Set up environment variables:
```bash
cp .env.example .env
# Edit .env with your configuration
```

4. Generate protobuf files (if modified):
```bash
protoc --go_out=. --go-grpc_out=. proto/auth/v1/auth.proto
```

## Configuration

Create a `.env` file with the following variables:

```env
# Database Configuration
DB_CONNECTION_STRING=user:password@tcp(localhost:3306)/authdb?charset=utf8mb4&parseTime=True&loc=Local

# JWT signing (RS256)
# Path to an RSA private key in PEM format (PKCS#1 or PKCS#8).
# - If the file does not exist, a 2048-bit dev key is generated and written
#   to this path (0600) on first use.
# - If the variable is unset entirely, an ephemeral in-memory dev key is
#   generated and a warning is logged; tokens will not survive a restart.
#   Do not run production without a persistent key file.
JWT_PRIVATE_KEY_FILE=./jwt_private_key.pem

# Admin gate for RegisterClient / ChangeClientSecret.
# These RPCs fail closed if ADMIN_SECRET is unset.
ADMIN_SECRET=choose-a-long-random-admin-secret

# TLS on the gRPC listener.
# Both must be set unless ENV=dev, in which case plaintext is allowed.
TLS_CERT_FILE=/path/to/server.crt
TLS_KEY_FILE=/path/to/server.key
ENV=dev

# Server Configuration
SERVER_PORT=8080
```

## Token Design

- **Access token**: RS256-signed JWT, 30-minute expiry. Claims: `sub`
  (user_id), `client_id`, `session_id`, `iat`, `exp`, `iss`. No refresh
  token, username, or email in the payload. `ValidateToken` is
  revocation-aware: it also checks that the session row still exists.
- **Refresh token**: opaque `"<session_id>.<secret>"`, where `secret` is
  256 bits of randomness (hex). The `session_id` prefix is the indexed
  lookup key (the sessions primary key); only a bcrypt hash of the secret
  half is stored. Tokens are rotated on every `RefreshToken` call with a
  sliding 7-day expiry. Presenting a rotated (stale) token is treated as a
  theft signal and revokes the entire session.
- **Multi-session**: each login creates a new session row keyed by a
  `session_id` UUID; users may hold many concurrent sessions per client.
- **Client credentials**: every RPC (except `HealthCheck`) requires
  `client_id` + `client_secret`, verified against a bcrypt hash in constant
  time. Client secrets are shown once at registration/rotation.

## Rate Limits (in-memory token buckets)

- Login (`GetToken`): 5 attempts / email / client / 15 minutes
- Registration (`RegisterUser`): 10 / client / hour
- Per-client ceiling: 1000 requests / minute

Buckets are per-process; move to Redis before replicating the service.

## Database Setup

The service will automatically create the required tables on startup. Ensure your MySQL database is running and accessible.

### Tables Created:
- `users`: User information and credentials
- `clients`: Registered client applications
- `sessions`: User sessions and refresh tokens

## Running the Service

```bash
go run cmd/server/main.go
```

The server will start on port 8080 (or the port specified in your environment).

## API Documentation

### gRPC Service: AuthService

#### 1. Health Check
```protobuf
rpc HealthCheck(google.protobuf.Empty) returns (HealthCheckResponse);
```
**Purpose**: Check service health and availability.

**Response**:
- `status`: Service status (SERVING, NOT_SERVING, SERVICE_UNKNOWN)
- `message`: Status message
- `details`: Additional service information

#### 2. Register Client
```protobuf
rpc RegisterClient(RegisterClientRequest) returns (RegisterClientResponse);
```
**Purpose**: Register a new client application.

**Request**:
- `client_name`: Name of the client application

**Response**:
- `success`: Operation success status
- `message`: Response message
- `client_id`: Generated client ID (UUID)
- `client_secret`: Generated client secret

#### 3. Register User
```protobuf
rpc RegisterUser(RegisterUserRequest) returns (RegisterUserResponse);
```
**Purpose**: Register a new user account.

**Request**:
- `username`: User's display name
- `email`: User's email address (must be unique)
- `password`: User's password (minimum 8 characters)
- `client_id`: Client ID the user belongs to

**Response**:
- `success`: Operation success status
- `message`: Response message
- `user_id`: Generated user ID (UUID)

#### 4. Login User
```protobuf
rpc LoginUser(LoginUserRequest) returns (LoginUserResponse);
```
**Purpose**: Authenticate user and create session.

**Request**:
- `email`: User's email address
- `password`: User's password
- `client_id`: Client ID
- `user_agent`: Optional user agent string

**Response**:
- `success`: Operation success status
- `message`: Response message
- `access_token`: JWT access token (24-hour expiry)
- `refresh_token`: Refresh token (7-day expiry)
- `expires_at`: Token expiration timestamp
- `user`: User profile information

#### 5. Validate Token
```protobuf
rpc ValidateToken(ValidateTokenRequest) returns (ValidateTokenResponse);
```
**Purpose**: Validate JWT access token.

**Request**:
- `access_token`: JWT token to validate

**Response**:
- `valid`: Token validity status
- `message`: Validation message
- `user_id`: User ID from token (if valid)
- `expires_at`: Token expiration timestamp

#### 6. Refresh Token
```protobuf
rpc RefreshToken(RefreshTokenRequest) returns (RefreshTokenResponse);
```
**Purpose**: Refresh access token using refresh token.

**Request**:
- `refresh_token`: Valid refresh token
- `client_id`: Client ID

**Response**:
- `success`: Operation success status
- `message`: Response message
- `access_token`: New JWT access token
- `refresh_token`: New refresh token
- `expires_at`: New token expiration timestamp

#### 7. Logout User
```protobuf
rpc LogoutUser(LogoutUserRequest) returns (LogoutUserResponse);
```
**Purpose**: Logout user and invalidate session.

**Request**:
- `refresh_token`: Refresh token to invalidate

**Response**:
- `success`: Operation success status
- `message`: Response message

#### 8. Get User Profile
```protobuf
rpc GetUserProfile(GetUserProfileRequest) returns (GetUserProfileResponse);
```
**Purpose**: Retrieve user profile information.

**Request**:
- `access_token`: Valid JWT access token

**Response**:
- `success`: Operation success status
- `message`: Response message
- `user`: User profile information

## Usage Examples

### Testing with grpcurl

1. **Health Check**:
```bash
grpcurl -plaintext localhost:8080 auth.v1.AuthService/HealthCheck
```

2. **Register Client**:
```bash
grpcurl -plaintext -d '{"client_name": "My App"}' localhost:8080 auth.v1.AuthService/RegisterClient
```

3. **Register User**:
```bash
grpcurl -plaintext -d '{"username": "john_doe", "email": "john@example.com", "password": "password123", "client_id": "YOUR_CLIENT_ID"}' localhost:8080 auth.v1.AuthService/RegisterUser
```

4. **Login User**:
```bash
grpcurl -plaintext -d '{"email": "john@example.com", "password": "password123", "client_id": "YOUR_CLIENT_ID", "user_agent": "grpcurl"}' localhost:8080 auth.v1.AuthService/LoginUser
```

### Integration Flow

1. **Client Registration**: Register your application to get `client_id` and `client_secret`
2. **User Registration**: Users register with their credentials and your `client_id`
3. **Authentication**: Users login to receive `access_token` and `refresh_token`
4. **API Calls**: Include `access_token` in requests to protected endpoints
5. **Token Refresh**: Use `refresh_token` to get new tokens when access token expires
6. **Logout**: Invalidate session when user logs out

## Security Features

- **Password Hashing**: bcrypt with salt (user passwords, client secrets, refresh tokens)
- **JWT Tokens**: RS256 signed tokens, 30-minute expiry, revocation-aware validation
- **Session Management**: Multi-session per user with rotated refresh tokens; reuse of a rotated token revokes the session
- **Client Validation**: Multi-tenant support with client isolation; email uniqueness is scoped per client; client secret required on every RPC
- **Admin Gate**: `RegisterClient` / `ChangeClientSecret` require `ADMIN_SECRET`
- **Rate Limiting**: in-memory token buckets on login, registration, and per-client volume
- **TLS**: required on the listener unless `ENV=dev`
- **Input Validation**: Email format, password strength (≥8 chars, also on password change), required fields
- **No PII in logs**: user_ids and client_ids only — never emails or tokens
- **Automatic Cleanup**: Expired sessions are cleaned up hourly

## Error Handling

The service provides detailed error messages for:
- Invalid credentials
- Missing required fields
- Token validation failures
- Client authentication issues
- Database connectivity problems

## Monitoring

- **Logging**: Comprehensive request/response logging
- **Health Checks**: Built-in health endpoint
- **Metrics**: Request duration and method tracking via interceptors

## Development

### Adding New Endpoints

1. Update `proto/auth/v1/auth.proto`
2. Regenerate protobuf files: `protoc --go_out=. --go-grpc_out=. proto/auth/v1/auth.proto`
3. Implement method in `pkg/service/auth_service.go`
4. Add repository methods if needed in `pkg/repository/auth_repository.go`

### Database Migrations

The service uses GORM AutoMigrate. To modify schemas:
1. Update models in `pkg/models/model.go`
2. Restart the service to apply changes

## License

[License information]

## Contributing

[Contributing guidelines]
