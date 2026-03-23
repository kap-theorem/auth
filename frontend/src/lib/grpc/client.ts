import * as grpc from '@grpc/grpc-js';
import * as protoLoader from '@grpc/proto-loader';
import path from 'path';

let clientInstance: any = null;

export function getAuthClient() {
    if (clientInstance) return clientInstance;

    // In a Docker standalone container, the path is often /app/src/lib/grpc/auth.proto
    // We check a few known locations since Next.js alters the execution context
    let protoPath = '';
    const possiblePaths = [
        path.join(process.cwd(), 'src', 'lib', 'grpc', 'auth.proto'),                       // Local dev
        path.join(process.cwd(), '.next', 'server', 'src', 'lib', 'grpc', 'auth.proto'),    // NextJS Output
        '/app/src/lib/grpc/auth.proto',                                                     // Explicit Docker
    ];

    console.log(`[gRPC] process.cwd(): ${process.cwd()}`);
    for (const p of possiblePaths) {
        try {
            const exists = require('fs').existsSync(p);
            console.log(`[gRPC] Checking path: ${p} - Exists: ${exists}`);
            if (exists) {
                protoPath = p;
                break;
            }
        } catch (e) {
            console.error(`[gRPC] Error checking path ${p}:`, e);
        }
    }

    if (!protoPath) {
        console.error('[gRPC] CRITICAL ERROR: Could not locate auth.proto on the filesystem.');
        throw new Error('auth.proto not found');
    }

    try {
        console.log(`[gRPC] Loading proto from: ${protoPath}`);
        const packageDefinition = protoLoader.loadSync(protoPath, {
            keepCase: true,
            longs: String,
            enums: String,
            defaults: true,
            oneofs: true,
        });

        const protoDescriptor = grpc.loadPackageDefinition(packageDefinition) as any;
        const authPackage = protoDescriptor.auth.v1;

        const GRPC_SERVER_URL = process.env.GRPC_SERVER_URL || 'localhost:50051';

        clientInstance = new authPackage.AuthService(
            GRPC_SERVER_URL,
            process.env.GRPC_USE_TLS === 'true'
                ? grpc.credentials.createSsl()
                : grpc.credentials.createInsecure()
        );
    } catch (err: any) {
        console.error('[gRPC] CRITICAL FAILURE during proto load:', err);
        // Log stack trace if available
        if (err.stack) console.error(err.stack);
        throw err;
    }

    return clientInstance;
}
