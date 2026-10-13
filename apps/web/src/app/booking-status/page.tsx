import type { Metadata } from "next";
import BookingStatus from "@/components/booking-status";

export const metadata: Metadata = {
  title: "Booking Request Status | MMSU Tennis Club",
  description: "Privately check the status of your MMSU Tennis Club training request.",
  robots: { index: false, follow: false },
};

export default function BookingStatusPage() {
  return <BookingStatus />;
}
