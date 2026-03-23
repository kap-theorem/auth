import { NextResponse } from 'next/server';
import { getAuthClient } from '@/lib/grpc/client';

export const dynamic = 'force-dynamic';

export async function GET() {
    const authServiceClient = getAuthClient();

    return new Promise<NextResponse>((resolve) => {
        authServiceClient.ListClients({}, (err: any, response: any) => {
            if (err) {
                console.error('[API] ListClients gRPC Error:', err);
                resolve(NextResponse.json(
                    { success: false, message: 'Failed to fetch clients' },
                    { status: 500 }
                ));
                return;
            }

            resolve(NextResponse.json({
                success: true,
                clients: response.clients || []
            }));
        });
    });
}
