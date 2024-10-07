import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import faLocale from "dayjs/locale/fa";

dayjs.extend(relativeTime);

export function dateFromNow(date: Date | string) {
  return dayjs(date).locale("fa", faLocale).fromNow();
}
