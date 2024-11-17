import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import faLocale from "dayjs/locale/fa";

dayjs.extend(relativeTime);

export function dateFromNow(date: Date | string) {
  return dayjs(date).locale("fa", faLocale).fromNow();
}

export function isGregorianStartDateTime(date: Date | string) {
  const targetDate = new Date(date);

  return (
    targetDate.getUTCFullYear() === 1 &&
    targetDate.getUTCMonth() === 0 &&
    targetDate.getUTCDay() === 1 &&
    targetDate.getUTCHours() === 0 &&
    targetDate.getUTCMinutes() === 0 &&
    targetDate.getUTCSeconds() === 0
  );
}
