import { NextResponse } from 'next/server';
import { getAuthClient } from '@/lib/grpc/client';

export const dynamic = 'force-dynamic';

export async function GET(
    request: Request,
    { params }: { params: Promise<{ clientId: string }> }
) {
    const { clientId } = await params;
    const authServiceClient = getAuthClient();

    return new Promise<NextResponse>((resolve) => {
        authServiceClient.GetClientStats({ client_id: clientId }, (err: any, response: any) => {
            if (err) {
                console.error(`[API] GetClientStats gRPC Error for ${clientId}:`, err);
                resolve(NextResponse.json(
                    { success: false, message: 'Failed to fetch application stats' },
                    { status: 500 }
                ));
                return;
            }

            resolve(NextResponse.json({
                success: true,
                stats: {
                    totalUsers: response.total_users,
                    last24hLogins: response.last_24h_logins || 0
                }
            }));
        });
    });
}
