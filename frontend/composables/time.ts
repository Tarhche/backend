import {useDayjs} from '#dayjs'

const dayjs = useDayjs()

// Global configurations
dayjs.locale('fa')

function isZeroDate(date: string): boolean {
    const zeroDate = '0001-01-01T00:00:00Z'

    return date === zeroDate
}

function toISOString(date: string): string {
    return new Date(date).toISOString()
}

function toFormat(date: string, format: string): string {
    return dayjs(date).format(format)
}

function toAgo(date: string): string {
    return dayjs(date).fromNow()
}

export function useTime() {
    return {
        isZeroDate: isZeroDate,
        toISOString: toISOString,
        toFormat: toFormat,
        toAgo: toAgo,
    }
}