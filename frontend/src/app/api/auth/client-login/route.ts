import { NextResponse } from 'next/server';
import { cookies } from 'next/headers';
import { getAuthClient } from '@/lib/grpc/client';

export const dynamic = 'force-dynamic';

export async function POST(request: Request) {
    const authServiceClient = getAuthClient();
    try {
        const body = await request.json();
        const { clientId, clientSecret } = body;

        if (!clientId || !clientSecret) {
            return NextResponse.json(
                { success: false, message: 'Client ID and Client Secret are required' },
                { status: 400 }
            );
        }

        return new Promise<NextResponse>((resolve) => {
            authServiceClient.ValidateClientCredentials({ client_id: clientId, client_secret: clientSecret }, async (err: any, response: any) => {
                if (err) {
                    console.error('gRPC Error:', err);
                    resolve(NextResponse.json(
                        { success: false, message: 'Authentication service unavailable' },
                        { status: 500 }
                    ));
                    return;
                }

                if (response.valid) {
                    // Create an encrypted, HttpOnly session cookie to store the validated client ID
                    // In a production scenario, you might want to use a JWT here
                    const cookieStore = await cookies();

                    // Security: We prefix it to make its usage clear. We set it accessible only via HTTP,
                    // valid across the kaplabs domain, and expiring in 24 hours.
                    cookieStore.set('client_admin_session', clientId, {
                        httpOnly: true,
                        secure: process.env.NODE_ENV === 'production',
                        sameSite: 'lax',
                        domain: process.env.NODE_ENV === 'production' ? '.kaplabs.dev' : undefined,
                        maxAge: 60 * 60 * 24, // 24 hours
                        path: '/',
                    });

                    resolve(NextResponse.json(
                        { success: true, message: 'Client authenticated successfully', clientName: response.clientName },
                        { status: 200 }
                    ));
                } else {
                    resolve(NextResponse.json(
                        { success: false, message: response.message || 'Invalid credentials' },
                        { status: 401 }
                    ));
                }
            });
        });

    } catch (error) {
        console.error('Client Login Error:', error);
        return NextResponse.json(
            { success: false, message: 'Internal server error' },
            { status: 500 }
        );
    }
}
