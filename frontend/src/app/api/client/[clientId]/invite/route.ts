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
        authServiceClient.CreateInviteToken({
            client_id: clientId,
            email: body.email || '',
        }, (err: any, response: any) => {
            if (err) {
                console.error(`[API] CreateInviteToken gRPC Error for ${clientId}:`, err);
                resolve(NextResponse.json(
                    { success: false, message: 'Failed to create invite token' },
                    { status: 500 }
                ));
                return;
            }

            resolve(NextResponse.json({
                success: response.success,
                message: response.message,
                inviteToken: response.invite_token,
            }));
        });
    });
}
