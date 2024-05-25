<template>
    <ul v-if="pages>1" class="pagination justify-content-center">
        <li v-if="params.current != 1" class="page-item">
            <a @click.prevent="paginate(params.current-1)" class="page-link" :href="`?page=${params.current-1}`" aria-label="Previous">
                <span aria-hidden="true">&laquo;</span>
            </a>
        </li>
        <li v-for="page of params.pages" :key="page" class="page-item">
            <a @click.prevent="paginate(page)" :class="{active: page==params.current}" class="page-link" :href="`?page=${page}`">{{ page }}</a>
        </li>
        <li v-if="params.current != params.pages" class="page-item">
            <a @click.prevent="paginate(params.current+1)" class="page-link" :href="`?page=${params.current+1}`" aria-label="Next">
                <span aria-hidden="true">&raquo;</span>
            </a>
        </li>
    </ul>
</template>

<script lang="ts" setup>
const emit = defineEmits(['paginate'])

const props = defineProps({
	"pages": {
        type: Number,
        default: 1,
    },
	"current": {
        type: Number,
        default: 1,
    },
})

const params = reactive({
    pages: props.pages,
    current: props.current,
})

function paginate(page: number) {
    params.current = page;

    emit('paginate', page)
}
</script>