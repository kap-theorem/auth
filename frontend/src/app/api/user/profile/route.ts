import { NextResponse } from 'next/server';
import { cookies } from 'next/headers';
import { getAuthClient } from '@/lib/grpc/client';

export const dynamic = 'force-dynamic';

export async function GET() {
    const cookieStore = await cookies();
    const accessToken = cookieStore.get('access_token')?.value;

    if (!accessToken) {
        return NextResponse.json(
            { success: false, message: 'Not authenticated' },
            { status: 401 }
        );
    }

    const authServiceClient = getAuthClient();

    return new Promise<NextResponse>((resolve) => {
        authServiceClient.ValidateToken({ access_token: accessToken }, (err: any, response: any) => {
            if (err || !response.valid) {
                console.error('[API] Profile validation failed:', err || response?.message);
                resolve(NextResponse.json(
                    { success: false, message: 'Invalid or expired session' },
                    { status: 401 }
                ));
                return;
            }

            resolve(NextResponse.json({
                success: true,
                user: {
                    userId: response.user.user_id,
                    username: response.user.username,
                    email: response.user.email,
                    clientId: response.user.client_id,
                    createdAt: response.user.created_at
                }
            }));
        });
    });
}
