"use client";
import {DateTimePicker, DatesProvider} from "@mantine/dates";
import "@mantine/dates/styles.css";
import "dayjs/locale/fa";

type Props = {
  initialDate?: string;
};

export function DateField({initialDate}: Props) {
  const date =
    initialDate === undefined
      ? null
      : new Date(initialDate).getDate() === 1
        ? null
        : new Date(initialDate);

  return (
    <DatesProvider
      settings={{
        locale: "fa",
      }}
    >
      <DateTimePicker
        valueFormat="DD MMM YYYY hh:mm A"
        placeholder="تاریخ انتشار را وارد کنید"
        label="تاریخ تایید"
        name="approvalDate"
        defaultValue={date}
        clearable
      />
    </DatesProvider>
  );
}
