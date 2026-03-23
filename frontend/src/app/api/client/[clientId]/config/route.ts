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
        authServiceClient.GetClientConfig({ client_id: clientId }, (err: any, response: any) => {
            if (err) {
                console.error(`[API] GetClientConfig gRPC Error for ${clientId}:`, err);
                resolve(NextResponse.json(
                    { success: false, message: 'Failed to fetch application config' },
                    { status: 500 }
                ));
                return;
            }

            resolve(NextResponse.json({
                success: true,
                config: response.config || { demoMode: false, inviteOnly: false, loginType: "both" }
            }));
        });
    });
}

export async function POST(
    request: Request,
    { params }: { params: Promise<{ clientId: string }> }
) {
    const { clientId } = await params;
    const body = await request.json();
    const authServiceClient = getAuthClient();

    return new Promise<NextResponse>((resolve) => {
        authServiceClient.UpdateClientConfig({
            client_id: clientId,
            config: {
                demo_mode: body.demoMode,
                invite_only: body.inviteOnly,
                login_type: body.loginType
            }
        }, (err: any, response: any) => {
            if (err || !response.success) {
                console.error(`[API] UpdateClientConfig gRPC Error for ${clientId}:`, err);
                resolve(NextResponse.json(
                    { success: false, message: 'Failed to update application config' },
                    { status: 500 }
                ));
                return;
            }

            resolve(NextResponse.json({
                success: true,
            }));
        });
    });
}
