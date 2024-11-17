"use client";
import {forwardRef} from "react";
import {
  DateTimePicker,
  DatesProvider,
  DateTimePickerProps,
} from "@mantine/dates";
import "@mantine/dates/styles.css";
import "dayjs/locale/fa";

export const DateTimeInput = forwardRef<HTMLButtonElement, DateTimePickerProps>(
  (props, ref) => {
    return (
      <DatesProvider
        settings={{
          locale: "fa",
        }}
      >
        <DateTimePicker {...props} ref={ref} />
      </DatesProvider>
    );
  },
);

DateTimeInput.displayName = "DateTimeInput";
