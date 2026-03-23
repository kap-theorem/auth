import { NextResponse } from 'next/server';
import { getAuthClient } from '@/lib/grpc/client';

export const dynamic = 'force-dynamic';

export async function PATCH(
    request: Request,
    { params }: { params: Promise<{ clientId: string; userId: string }> }
) {
    const { clientId, userId } = await params;
    const body = await request.json();
    const authServiceClient = getAuthClient();

    // Use UpdateUserProfile with an admin-scoped call
    // We pass the target user_id alongside new_username and new_email
    return new Promise<NextResponse>((resolve) => {
        authServiceClient.UpdateClientUser({
            client_id: clientId,
            user_id: userId,
            new_username: body.username || '',
            new_email: body.email || '',
            new_password: body.password || '',
            lock_username: body.lockUsername || false,
            lock_email: body.lockEmail || false,
            lock_password: body.lockPassword || false,
        }, (err: any, response: any) => {
            if (err) {
                console.error(`[API] UpdateClientUser gRPC Error:`, err);
                resolve(NextResponse.json(
                    { success: false, message: err.message || 'Failed to update user' },
                    { status: 500 }
                ));
                return;
            }

            if (!response.success) {
                resolve(NextResponse.json(
                    { success: false, message: response.message || 'Failed to update user' },
                    { status: 400 }
                ));
                return;
            }

            resolve(NextResponse.json({ success: true }));
        });
    });
}
