import type { Metadata } from "next";
import { JetBrains_Mono, Manrope } from "next/font/google";
import "./globals.css";

const manrope = Manrope({
	subsets: ["latin"],
	weight: ["200", "300", "400", "500", "600", "700"],
	variable: "--font-display",
});

const jetBrainsMono = JetBrains_Mono({
	subsets: ["latin"],
	weight: ["300", "400", "500"],
	variable: "--font-mono",
});

export const metadata: Metadata = {
	title: "SpaceScale | Smarter Deployment Platform",
	description: "Deploy workloads with maximum scale, simplicity, and security.",
};

export default function RootLayout({
	children,
}: Readonly<{
	children: React.ReactNode;
}>) {
	return (
		<html lang="en" suppressHydrationWarning>
			<head>
				<link
					rel="stylesheet"
					href="https://fonts.googleapis.com/css2?family=Material+Symbols+Outlined:wght,FILL@100..700,0..1&display=swap"
				/>
			</head>
			<body
				className={`${manrope.variable} ${jetBrainsMono.variable} font-display`}
			>
				{children}
			</body>
		</html>
	);
}
