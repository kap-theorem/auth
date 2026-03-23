import { NextRequest, NextResponse } from "next/server";
import { getAuthClient } from "@/lib/grpc/client";

export const dynamic = 'force-dynamic';

export async function POST(req: NextRequest): Promise<NextResponse> {
    const authServiceClient = getAuthClient();
    try {
        const { resetToken, newPassword } = await req.json();

        if (!resetToken || !newPassword) {
            return NextResponse.json({ success: false, message: "Reset token and new password are required" }, { status: 400 });
        }

        return new Promise<NextResponse>((resolve) => {
            authServiceClient.ResetPassword(
                { reset_token: resetToken, new_password: newPassword },
                (error: any, response: any) => {
                    if (error) {
                        console.error("gRPC ResetPassword Error:", error);
                        resolve(NextResponse.json({ success: false, message: error.message || "Internal server error" }, { status: 500 }));
                        return;
                    }
                    resolve(NextResponse.json({
                        success: response.success,
                        message: response.message,
                    }));
                }
            );
        });
    } catch {
        return NextResponse.json({ success: false, message: "Invalid request payload" }, { status: 400 });
    }
}
