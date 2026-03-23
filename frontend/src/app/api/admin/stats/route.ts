import { NextResponse } from 'next/server';
import { getAuthClient } from '@/lib/grpc/client';

export const dynamic = 'force-dynamic';

export async function GET() {
    const authServiceClient = getAuthClient();

    return new Promise<NextResponse>((resolve) => {
        authServiceClient.GetSystemStats({}, (err: any, response: any) => {
            if (err) {
                console.error('[API] GetSystemStats gRPC Error:', err);
                resolve(NextResponse.json(
                    { success: false, message: 'Failed to fetch system stats' },
                    { status: 500 }
                ));
                return;
            }

            resolve(NextResponse.json({
                success: true,
                stats: {
                    totalUsers: response.total_users,
                    totalClients: response.total_clients,
                    activeSessions: response.active_sessions,
                    systemHealth: response.system_health
                }
            }));
        });
    });
}
