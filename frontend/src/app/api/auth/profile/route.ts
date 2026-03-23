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

        const { newUsername, newEmail } = await req.json();

        return new Promise<NextResponse>((resolve) => {
            authServiceClient.UpdateUserProfile(
                { access_token: accessToken, new_username: newUsername || "", new_email: newEmail || "" },
                (error: any, response: any) => {
                    if (error) {
                        resolve(NextResponse.json({ success: false, message: error.message || "Internal server error" }, { status: 500 }));
                        return;
                    }
                    if (response.success) {
                        resolve(NextResponse.json(response));
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
