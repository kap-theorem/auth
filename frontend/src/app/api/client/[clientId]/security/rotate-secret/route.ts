import { NextResponse } from 'next/server';
import { getAuthClient } from '@/lib/grpc/client';

export const dynamic = 'force-dynamic';

export async function POST(
    request: Request,
    { params }: { params: Promise<{ clientId: string }> }
) {
    const { clientId } = await params;
    const body = await request.json().catch(() => ({}));
    const authServiceClient = getAuthClient();

    return new Promise<NextResponse>((resolve) => {
        authServiceClient.ChangeClientSecret({
            client_id: clientId,
            current_secret: body.currentSecret,
            new_secret: body.newSecret || '',
        }, (err: any, response: any) => {
            if (err) {
                console.error(`[API] ChangeClientSecret gRPC Error for ${clientId}:`, err);
                resolve(NextResponse.json(
                    { success: false, message: 'Failed to rotate client secret' },
                    { status: 500 }
                ));
                return;
            }

            resolve(NextResponse.json(response));
        });
    });
}
