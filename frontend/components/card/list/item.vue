<script>
    export default {
        props: ['href', 'title', 'excerpt']
    }
</script>

<template>
    <li>
        <span class="py-1">
            <h6 class="fw-bold">
                <a :href="href" class="text-dark">{{ title }}</a>
            </h6>
            <p class="text-muted">{{ excerpt }}</p>
        </span>
    </li>
</template>