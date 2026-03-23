import { NextResponse } from 'next/server';
import { getAuthClient } from '@/lib/grpc/client';

export async function POST(request: Request) {
    try {
        const body = await request.json();
        const { clientName } = body;

        if (!clientName) {
            return NextResponse.json(
                { success: false, message: 'Application name is required' },
                { status: 400 }
            );
        }

        const authServiceClient = getAuthClient();

        return new Promise<NextResponse>((resolve) => {
            authServiceClient.RegisterClient({ client_name: clientName }, (err: any, response: any) => {
                if (err) {
                    console.error('[API] RegisterClient gRPC Error:', err);
                    resolve(NextResponse.json(
                        { success: false, message: 'Authentication service unavailable' },
                        { status: 500 }
                    ));
                    return;
                }

                if (response.success) {
                    resolve(NextResponse.json({
                        success: true,
                        message: response.message,
                        clientId: response.client_id,
                        clientSecret: response.client_secret
                    }, { status: 201 }));
                } else {
                    resolve(NextResponse.json(
                        { success: false, message: response.message || 'Failed to register application' },
                        { status: 400 }
                    ));
                }
            });
        });

    } catch (error) {
        console.error('Client Registration API Error:', error);
        return NextResponse.json(
            { success: false, message: 'Internal server error' },
            { status: 500 }
        );
    }
}
