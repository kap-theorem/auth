import { NextRequest, NextResponse } from 'next/server';
import { getAuthClient } from '@/lib/grpc/client';

export const dynamic = 'force-dynamic';

export async function GET(
    request: NextRequest,
    { params }: { params: Promise<{ clientId: string }> }
) {
    const { clientId } = await params;
    const authServiceClient = getAuthClient();

    const accessToken = request.cookies.get('admin_access_token')?.value
        || request.cookies.get('client_admin_session')?.value;

    if (!accessToken) {
        return NextResponse.json(
            { success: false, message: 'Unauthorized' },
            { status: 401 }
        );
    }

    const searchParams = new URL(request.url).searchParams;
    const days = parseInt(searchParams.get('days') || '30');

    return new Promise<NextResponse>((resolve) => {
        authServiceClient.GetClientLoginActivity({ client_id: clientId, days }, (err: any, response: any) => {
            if (err) {
                console.error(`[API] GetClientLoginActivity gRPC Error for ${clientId}:`, err);
                resolve(NextResponse.json(
                    { success: false, message: 'Failed to fetch login activity' },
                    { status: 500 }
                ));
                return;
            }

            resolve(NextResponse.json(response));
        });
    });
}
