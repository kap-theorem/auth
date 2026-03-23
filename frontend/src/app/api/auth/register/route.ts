import { NextRequest, NextResponse } from "next/server";
import { getAuthClient } from "@/lib/grpc/client";

export const dynamic = 'force-dynamic';

export async function POST(req: NextRequest): Promise<NextResponse> {
    const authServiceClient = getAuthClient();
    try {
        const { username, email, password, clientId } = await req.json();

        if (!username || !email || !password || !clientId) {
            return NextResponse.json({ success: false, message: "Missing required fields" }, { status: 400 });
        }

        return new Promise<NextResponse>((resolve) => {
            authServiceClient.RegisterUser(
                { username, email, password, client_id: clientId },
                (error: any, response: any) => {
                    if (error) {
                        console.error("gRPC Register Error:", error);
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
