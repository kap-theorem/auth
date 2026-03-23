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
        authServiceClient.ListClientUsers({ client_id: clientId }, (err: any, response: any) => {
            if (err) {
                console.error(`[API] ListClientUsers gRPC Error for ${clientId}:`, err);
                resolve(NextResponse.json(
                    { success: false, message: 'Failed to fetch application users' },
                    { status: 500 }
                ));
                return;
            }

            resolve(NextResponse.json({
                success: true,
                users: response.users || []
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
        authServiceClient.CreateClientUser({
            client_id: clientId,
            username: body.username,
            email: body.email,
            password: body.password
        }, (err: any, response: any) => {
            if (err || !response.success) {
                console.error(`[API] CreateClientUser gRPC Error for ${clientId}:`, err);
                resolve(NextResponse.json(
                    { success: false, message: response?.message || 'Failed to create user' },
                    { status: 400 }
                ));
                return;
            }

            resolve(NextResponse.json({
                success: true,
                user: response.user
            }));
        });
    });
}

export async function DELETE(
    request: Request,
    { params }: { params: Promise<{ clientId: string }> }
) {
    const { clientId } = await params;
    const { searchParams } = new URL(request.url);
    const userId = searchParams.get('userId');

    if (!userId) {
        return NextResponse.json({ success: false, message: 'Missing userId' }, { status: 400 });
    }

    const authServiceClient = getAuthClient();

    return new Promise<NextResponse>((resolve) => {
        authServiceClient.DeleteClientUser({ client_id: clientId, user_id: userId }, (err: any) => {
            if (err) {
                console.error(`[API] DeleteClientUser gRPC Error for ${clientId}:`, err);
                resolve(NextResponse.json(
                    { success: false, message: 'Failed to delete user' },
                    { status: 500 }
                ));
                return;
            }

            resolve(NextResponse.json({ success: true }));
        });
    });
}
