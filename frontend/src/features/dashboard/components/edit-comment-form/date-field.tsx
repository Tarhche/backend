"use client";
import {DatePickerInput, DatesProvider} from "@mantine/dates";
import "@mantine/dates/styles.css";
import "dayjs/locale/fa";

type Props = {
  initialDate?: string;
};

export function DateField({initialDate}: Props) {
  const date = initialDate === undefined ? null : new Date(initialDate);
  return (
    <DatesProvider
      settings={{
        locale: "fa",
      }}
    >
      <DatePickerInput
        defaultValue={date}
        label="تاریخ تایید"
        name="approvalDate"
        clearable
      />
    </DatesProvider>
  );
}
