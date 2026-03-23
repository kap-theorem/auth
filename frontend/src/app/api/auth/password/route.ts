import { NextRequest, NextResponse } from "next/server";
import { getAuthClient } from "@/lib/grpc/client";

export const dynamic = 'force-dynamic';

export async function POST(req: NextRequest): Promise<NextResponse> {
    const authServiceClient = getAuthClient();
    try {
        const accessToken = req.cookies.get('access_token')?.value;
        if (!accessToken) {
            return NextResponse.json({ success: false, message: "Unauthorized" }, { status: 401 });
        }

        const { currentPassword, newPassword } = await req.json();

        return new Promise<NextResponse>((resolve) => {
            authServiceClient.ChangeUserPassword(
                { access_token: accessToken, current_password: currentPassword, new_password: newPassword },
                (error: any, response: any) => {
                    if (error) {
                        resolve(NextResponse.json({ success: false, message: error.message || "Internal server error" }, { status: 500 }));
                        return;
                    }
                    if (response.success) {
                        const res = NextResponse.json(response);
                        // Backend invalidates sessions on password change, so we must clear frontend cookies
                        res.cookies.delete('access_token');
                        res.cookies.delete('refresh_token');
                        resolve(res);
                    } else {
                        resolve(NextResponse.json({ success: false, message: response.message }, { status: 400 }));
                    }
                }
            );
        });
    } catch (error) {
        return NextResponse.json({ success: false, message: "Invalid request payload" }, { status: 400 });
    }
}
