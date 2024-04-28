<template>

<div class="container">
        <div class="row">
            <dashboardSidebar class="col-md-3 ml-sm-auto"/>
            <main class="col-md-9 ml-sm-auto">
                <!-- 
                  <nav aria-label="breadcrumb">
                    <ol class="breadcrumb">
						<li class="breadcrumb-item"><a href="#">Home</a></li>
						<li class="breadcrumb-item active" aria-current="page">Overview</li>
                    </ol>
                  </nav>
                  <h1 class="h2">Dashboard</h1>
                  <p>This is the homepage of a simple admin interface which is part of a tutorial written on Themesberg</p> 
                -->

                <div class="row">
                    <div class="col-12 mb-4 mb-lg-0">
                        <div class="card">
							<div class="card-header">جدیدترین مقالات</div>
                            <div class="card-body">
                                <div class="table-responsive">
                                    <table class="table table-striped table-borderless table-hover align-middle">
                                        <thead class="border-bottom">
                                          <tr>
                                            <th scope="col">#</th>
                                            <th scope="col">عنوان</th>
                                            <th scope="col">تاریخ انتشار</th>
                                            <th scope="col">#</th>
                                          </tr>
                                        </thead>
                                        <tbody v-if="!pending">
                                          <tr v-for="(article, index) in data.items" :key="index">
                                            <th scope="row">{{ index + 1 }}</th>
                                            <td>{{ article.title }}</td>
                                            <td>{{ article.published_at }}</td>
                                            <td>
												<NuxtLink :to="`/articles/${article.uuid}`" class="btn mx-1 btn-sm btn-primary">
													<span class="fa fa-eye"></span>
												</NuxtLink>
												<NuxtLink :to="`/dashboard/articles/edit/${article.uuid}`" class="btn mx-1 btn-sm btn-primary">
													<span class="fa fa-pen"></span>
												</NuxtLink>
												<button type="button" class="btn mx-1 btn-sm btn-danger">
													<span class="fa fa-trash"></span>
												</button>
											</td>
                                          </tr>
										  <tr v-if="data.items.length == 0">
											<td colspan="5">
												<p>هیچ مقاله ای وجود ندارد</p>
											</td>
										  </tr>
                                        </tbody>
                                      </table>
                                </div>
								<NuxtLink v-if="!pending && data.pagination.total_pages > 1" to="/dashboard/articles">مشاهده بیشتر</NuxtLink>
                            </div>
                        </div>
                    </div>
                </div>
            </main>
        </div>
    </div>
</template>

<script setup>
definePageMeta({
  layout: 'dashboard',
})

const { data, pending, error } = await useAsyncData(
	'dashboard.articles.index',
	useDashboardArticles().index
)

</script>